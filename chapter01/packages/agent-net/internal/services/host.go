// Host adapter: the few reads of the agent-host itself (privilege, files, name resolution),
// behind an interface so tests need no real host.
package services

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-net/internal/config"
)

// Host is the host surface the tool needs.
type Host interface {
	Geteuid() int
	ReadFile(path string) ([]byte, error)
	// SocketAccessible succeeds when a connection to the unix socket can be opened.
	SocketAccessible(path string) error
	// LookupIPv4 resolves a host name via the fixed resolvers, not the host's own resolver.
	LookupIPv4(ctx context.Context, host string) ([]string, error)
	// Readlink reads the target of a symbolic link (the namespace identity of /proc/<pid>/ns/net).
	Readlink(path string) (string, error)
}

// FileStater is implemented by hosts that can report the owner and mode of a file. It is optional so that the Host
// interface stays unchanged; a host without it skips the ownership check of the endpoints file (tests).
type FileStater interface {
	Stat(path string) (uid int, mode fs.FileMode, err error)
}

// FileInstaller is implemented by hosts that can atomically replace a file. Optional like FileStater; refresh_endpoints
// uses it to install the endpoints file.
type FileInstaller interface {
	// InstallFile creates the parent directory if missing (0755), writes data to a temp file in the same directory
	// with the given mode, syncs it and renames it over path.
	InstallFile(path string, data []byte, perm fs.FileMode) error
}

// OSHost is the real host.
type OSHost struct{}

func (OSHost) InstallFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".endpoints-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func (OSHost) Stat(path string) (int, fs.FileMode, error) { return config.StatFile(path) }

func (OSHost) Geteuid() int { return os.Geteuid() }

func (OSHost) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (OSHost) Readlink(path string) (string, error) { return os.Readlink(path) }

func (OSHost) SocketAccessible(path string) error {
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return err
	}
	return c.Close()
}

// LookupIPv4 resolves through the fixed resolvers, not the host's resolv.conf (which may point at a stub with no upstream).
func (OSHost) LookupIPv4(ctx context.Context, host string) ([]string, error) {
	return LookupIPv4Via(ctx, []string{
		net.JoinHostPort(config.ResolverPrimary, config.DNSPort),
		net.JoinHostPort(config.ResolverSecondary, config.DNSPort),
	}, host)
}

// lookupServerTimeout bounds the attempt on one resolver.
const lookupServerTimeout = 5 * time.Second

// LookupIPv4Via resolves host to IPv4 addresses, asking each server (host:port) in order. The first server that answers
// without error ends the search, even with an empty list (a name with only AAAA records).
func LookupIPv4Via(ctx context.Context, servers []string, host string) ([]string, error) {
	var failures []string
	for _, server := range servers {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("lookup %s: %w", host, err)
		}
		out, err := lookupIPv4On(ctx, server, host)
		if err == nil {
			return out, nil
		}
		if cerr := ctx.Err(); cerr != nil {
			return nil, fmt.Errorf("lookup %s: %w", host, cerr)
		}
		failures = append(failures, fmt.Sprintf("%s: %s", server, lookupReason(err)))
	}
	return nil, fmt.Errorf("lookup %s failed on every resolver (%s); udp/tcp port 53 to these resolvers must be reachable from the agent host",
		host, oneLine(strings.Join(failures, "; ")))
}

func lookupIPv4On(ctx context.Context, server, host string) ([]string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, server)
		},
	}
	ctx, cancel := context.WithTimeout(ctx, lookupServerTimeout)
	defer cancel()
	addrs, err := r.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, a := range addrs {
		if v4 := a.IP.To4(); v4 != nil {
			out = append(out, v4.String())
		}
	}
	return out, nil
}

// lookupReason drops the "lookup <name> on <resolv.conf server>" prefix of a net.DNSError, which names the host's resolver rather than the one asked.
func lookupReason(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.Err
	}
	return err.Error()
}

// oneLine keeps the error on a single line for stderr rendering.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
