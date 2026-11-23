package tests

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-exec/internal/engine"
)

// behavior scripts what a started fake container does.
type behavior struct {
	stdout, stderr string
	exit           int
	hang           bool // runs until killed (then exit 137)
	delay          time.Duration
	oom            bool
	before         func() // runs when the container starts (e.g. writes into out/)
}

type fakeCtr struct {
	id, name   string
	body       engine.CreateBody
	state      string
	killCh     chan struct{}
	killOnce   sync.Once
	waitCh     chan int
	events     chan engine.Event
	out, errw  io.Writer
	attachDone chan error
}

// fakeEngine implements engine.Engine in memory.
type fakeEngine struct {
	mu        sync.Mutex
	images    map[string]bool
	imageEnv  []string
	loads     int
	ctrs      map[string]*fakeCtr
	bodies    map[string]engine.CreateBody // by name, kept after removal
	started   []string
	removed   []string
	seq       int
	pingErr   error
	createErr func(name string) error
	mutate    func(*engine.Inspect)
	behave    func(name string) behavior
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{
		images: map[string]bool{},
		ctrs:   map[string]*fakeCtr{},
		bodies: map[string]engine.CreateBody{},
		behave: func(string) behavior { return behavior{} },
	}
}

func (f *fakeEngine) Ping(ctx context.Context) error { return f.pingErr }

func (f *fakeEngine) ImageInspect(ctx context.Context, id string) (*engine.ImageInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.images[id] {
		return nil, fmt.Errorf("%w: image %s", engine.ErrNotFound, id)
	}
	info := &engine.ImageInfo{ID: id}
	info.Config.Env = f.imageEnv
	return info, nil
}

// ImageLoad makes the image of the profile available (the fake does not read the tar).
func (f *fakeEngine) ImageLoad(ctx context.Context, tar io.Reader) error {
	if _, err := io.Copy(io.Discard, tar); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	f.images["sha256:"+strings.Repeat("a", 64)] = true
	return nil
}

func (f *fakeEngine) ContainerCreate(ctx context.Context, name string, body engine.CreateBody) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		if err := f.createErr(name); err != nil {
			return "", err
		}
	}
	f.seq++
	c := f.newCtr(fmt.Sprintf("c%d", f.seq), name, body, "created")
	f.bodies[name] = body
	return c.id, nil
}

func (f *fakeEngine) newCtr(id, name string, body engine.CreateBody, state string) *fakeCtr {
	c := &fakeCtr{
		id: id, name: name, body: body, state: state,
		killCh: make(chan struct{}), waitCh: make(chan int, 1),
		events: make(chan engine.Event, 16), attachDone: make(chan error, 1),
	}
	f.ctrs[id] = c
	return c
}

// addExisting puts a container created by "another tool" into the engine.
func (f *fakeEngine) addExisting(name, state string, labels map[string]string, mem int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	body := engine.CreateBody{Labels: labels}
	body.HostConfig.Memory = mem
	f.newCtr(fmt.Sprintf("c%d", f.seq), name, body, state)
}

func matchLabels(labels map[string]string, filters []string) bool {
	for _, flt := range filters {
		k, v, hasV := strings.Cut(flt, "=")
		got, ok := labels[k]
		if !ok || (hasV && got != v) {
			return false
		}
	}
	return true
}

func (f *fakeEngine) ContainerList(ctx context.Context, labelFilters []string, all bool) ([]engine.ContainerSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []engine.ContainerSummary
	for _, c := range f.ctrs {
		if !matchLabels(c.body.Labels, labelFilters) {
			continue
		}
		out = append(out, engine.ContainerSummary{ID: c.id, Names: []string{"/" + c.name}, Labels: c.body.Labels, State: c.state})
	}
	return out, nil
}

func (f *fakeEngine) get(id string) (*fakeCtr, error) {
	c, ok := f.ctrs[id]
	if !ok {
		return nil, fmt.Errorf("%w: container %s", engine.ErrNotFound, id)
	}
	return c, nil
}

func (f *fakeEngine) ContainerInspect(ctx context.Context, id string) (*engine.Inspect, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.get(id)
	if err != nil {
		return nil, err
	}
	info := &engine.Inspect{ID: c.id, Name: "/" + c.name, Image: c.body.Image}
	info.Config.User = c.body.User
	info.Config.Env = c.body.Env
	info.Config.Cmd = c.body.Cmd
	info.Config.Entrypoint = c.body.Entrypoint
	info.Config.WorkingDir = c.body.WorkingDir
	info.Config.Labels = c.body.Labels
	info.Config.Tty = c.body.Tty
	info.Config.OpenStdin = c.body.OpenStdin
	info.HostConfig = c.body.HostConfig
	for _, m := range c.body.HostConfig.Mounts {
		info.Mounts = append(info.Mounts, engine.MountPoint{Type: "bind", Source: m.Source, Destination: m.Target, RW: !m.ReadOnly})
	}
	if f.mutate != nil {
		f.mutate(info)
	}
	return info, nil
}

func (f *fakeEngine) ContainerAttach(ctx context.Context, id string, stdout, stderr io.Writer) (<-chan error, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.get(id)
	if err != nil {
		return nil, err
	}
	c.out, c.errw = stdout, stderr
	return c.attachDone, nil
}

func (f *fakeEngine) ContainerStart(ctx context.Context, id string) error {
	f.mu.Lock()
	c, err := f.get(id)
	if err != nil {
		f.mu.Unlock()
		return err
	}
	c.state = "running"
	f.started = append(f.started, c.name)
	f.mu.Unlock()

	go func() {
		b := f.behave(c.name)
		if b.before != nil {
			b.before()
		}
		if b.stdout != "" {
			io.WriteString(c.out, b.stdout)
		}
		if b.stderr != "" {
			io.WriteString(c.errw, b.stderr)
		}
		code := b.exit
		if b.hang {
			select {
			case <-c.killCh:
				code = 137
			case <-time.After(10 * time.Second):
			}
		} else if b.delay > 0 {
			time.Sleep(b.delay)
		}
		if b.oom {
			c.events <- engine.Event{Type: "container", Action: "oom", ID: c.id}
			code = 137
		}
		c.events <- engine.Event{Type: "container", Action: "die", ID: c.id, Attributes: map[string]string{"exitCode": strconv.Itoa(code)}}
		f.mu.Lock()
		delete(f.ctrs, c.id) // AutoRemove
		f.mu.Unlock()
		c.attachDone <- nil
		c.waitCh <- code
	}()
	return nil
}

func (f *fakeEngine) ContainerWait(ctx context.Context, id string) (int, error) {
	f.mu.Lock()
	c, err := f.get(id)
	f.mu.Unlock()
	if err != nil {
		return 0, err
	}
	select {
	case code := <-c.waitCh:
		return code, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (f *fakeEngine) ContainerKill(ctx context.Context, id string) error {
	f.mu.Lock()
	c, ok := f.ctrs[id]
	f.mu.Unlock()
	if ok {
		c.killOnce.Do(func() { close(c.killCh) })
	}
	return nil
}

func (f *fakeEngine) ContainerRemove(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.ctrs[id]; ok {
		f.removed = append(f.removed, c.name)
		delete(f.ctrs, id)
	}
	return nil
}

func (f *fakeEngine) Events(ctx context.Context, id string) (<-chan engine.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.get(id)
	if err != nil {
		return nil, err
	}
	return c.events, nil
}

func (f *fakeEngine) remaining() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ctrs)
}

func (f *fakeEngine) startedNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.started...)
}
