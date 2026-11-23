// Package agentinit is the logic of agent-init, the supervisor tini starts in the agent container
// (USS-005). It runs as root, binds the authorization plugin socket, starts ephemeral-authz as user
// authz with that socket as fd 3, then starts dockerd. It restarts the plugin if it exits and exits
// with dockerd's exit code when dockerd exits.
//
// Processes are started through the Starter interface so tests run without root and without a real dockerd.
package agentinit

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Production values.
const (
	SocketPath  = "/run/docker/plugins/ephemeral-authz.sock"
	PluginPath  = "/usr/local/libexec/ephemeral-authz"
	PluginUser  = "authz"
	DockerdName = "dockerd"
	pluginPATH  = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

// Credential is the uid/gid a process runs as. Groups is cleared except for Gid.
type Credential struct{ Uid, Gid uint32 }

// Spec describes one process to start.
type Spec struct {
	Path       string
	Args       []string // arguments after the program name
	Env        []string // nil: inherit the environment of agent-init
	ExtraFiles []*os.File
	Cred       *Credential // nil: run as the caller (root)
}

// Process is a started process.
type Process interface {
	// Wait blocks until the process ends and returns its exit code (negative if not available).
	Wait() (int, error)
	Signal(sig os.Signal) error
}

// Starter starts processes. stdout and stderr are inherited from agent-init.
type Starter interface {
	Start(spec Spec) (Process, error)
}

// Init is the configuration of one agent-init run.
type Init struct {
	SocketPath string
	PluginPath string
	PluginUser string
	// DockerdPath is the dockerd binary; empty resolves "dockerd" with PATH.
	DockerdPath string
	Starter     Starter
	// LookupUser resolves PluginUser; nil uses /etc/passwd through os/user.
	LookupUser func(name string) (uid, gid uint32, err error)
	// NoCredential runs the plugin as the caller instead of PluginUser.
	NoCredential bool
	Log          io.Writer     // nil: stderr
	Backoff      time.Duration // pause before a plugin restart; 0: 1s
	TermWait     time.Duration // wait for the plugin after SIGTERM before SIGKILL; 0: 3s
}

func (c *Init) logf(format string, a ...any) {
	w := c.Log
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "agent-init: "+format+"\n", a...)
}

// listen creates the socket directory, removes a stale socket, binds and listens, mode 0600, owner root.
func (c *Init) listen() (*net.UnixListener, error) {
	if err := os.MkdirAll(filepath.Dir(c.SocketPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(c.SocketPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: c.SocketPath, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(c.SocketPath, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil // owner is the creator, root
}

func lookupUser(name string) (uint32, uint32, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	return uint32(uid), uint32(gid), nil
}

type exit struct {
	plugin bool
	code   int
}

// Run does the whole job and returns the process exit code. args are the command line arguments after
// the program name (none are accepted). sigs delivers SIGTERM/SIGINT, which are forwarded to dockerd.
func (c *Init) Run(args []string, sigs <-chan os.Signal) int {
	if len(args) > 0 {
		c.logf("unexpected arguments: %q", args)
		return 2
	}
	backoff, termWait := c.Backoff, c.TermWait
	if backoff <= 0 {
		backoff = time.Second
	}
	if termWait <= 0 {
		termWait = 3 * time.Second
	}

	var cred *Credential
	if !c.NoCredential {
		look := c.LookupUser
		if look == nil {
			look = lookupUser
		}
		uid, gid, err := look(c.PluginUser)
		if err != nil {
			c.logf("cannot look up user %s: %v", c.PluginUser, err)
			return 1
		}
		cred = &Credential{Uid: uid, Gid: gid}
	}

	l, err := c.listen()
	if err != nil {
		c.logf("cannot create socket %s: %v", c.SocketPath, err)
		return 1
	}
	defer l.Close()
	sockFile, err := l.File()
	if err != nil {
		c.logf("cannot get socket file: %v", err)
		return 1
	}
	defer sockFile.Close()

	pluginSpec := Spec{
		Path:       c.PluginPath,
		Env:        []string{"PATH=" + pluginPATH},
		ExtraFiles: []*os.File{sockFile}, // fd 3 in the plugin
		Cred:       cred,
	}

	exits := make(chan exit, 2)
	watch := func(p Process, plugin bool) {
		go func() {
			code, err := p.Wait()
			if err != nil {
				c.logf("wait: %v", err)
			}
			exits <- exit{plugin: plugin, code: code}
		}()
	}
	startPlugin := func() (Process, error) {
		p, err := c.Starter.Start(pluginSpec)
		if err != nil {
			return nil, err
		}
		watch(p, true)
		return p, nil
	}

	plugin, err := startPlugin()
	if err != nil {
		c.logf("cannot start %s: %v", c.PluginPath, err)
		return 1
	}
	pluginAlive := true
	c.logf("started %s", c.PluginPath)

	dockerd := c.DockerdPath
	if dockerd == "" {
		if dockerd, err = exec.LookPath(DockerdName); err != nil {
			c.logf("cannot find %s: %v", DockerdName, err)
			c.stopPlugin(plugin, pluginAlive, exits, termWait)
			return 1
		}
	}
	dp, err := c.Starter.Start(Spec{Path: dockerd})
	if err != nil {
		c.logf("cannot start %s: %v", dockerd, err)
		c.stopPlugin(plugin, pluginAlive, exits, termWait)
		return 1
	}
	watch(dp, false)
	c.logf("started %s", dockerd)

	var retry <-chan time.Time
	for {
		select {
		case sig := <-sigs:
			c.logf("forwarding %v to %s", sig, dockerd)
			if err := dp.Signal(sig); err != nil {
				c.logf("signal: %v", err)
			}
		case <-retry:
			retry = nil
			p, err := startPlugin()
			if err != nil {
				c.logf("cannot restart %s: %v", c.PluginPath, err)
				retry = time.After(backoff)
				continue
			}
			plugin, pluginAlive = p, true
			c.logf("restarted %s", c.PluginPath)
		case e := <-exits:
			if e.plugin {
				pluginAlive = false
				c.logf("%s exited (code %d), restarting in %s", c.PluginPath, e.code, backoff)
				retry = time.After(backoff)
				continue
			}
			code := e.code
			if code < 0 {
				code = 1
			}
			c.logf("%s exited (code %d)", dockerd, code)
			c.stopPlugin(plugin, pluginAlive, exits, termWait)
			return code
		}
	}
}

// stopPlugin sends SIGTERM to a running plugin and waits for it, then SIGKILL after termWait.
func (c *Init) stopPlugin(p Process, alive bool, exits <-chan exit, termWait time.Duration) {
	if !alive {
		return
	}
	_ = p.Signal(syscall.SIGTERM)
	select {
	case <-exits:
		return
	case <-time.After(termWait):
	}
	c.logf("plugin did not stop after SIGTERM, killing it")
	_ = p.Signal(syscall.SIGKILL)
	select {
	case <-exits:
	case <-time.After(termWait):
	}
}
