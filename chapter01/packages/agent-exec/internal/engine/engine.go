// Package engine is the Docker Engine API as run_ephemeral_container uses it (IP-INCONTAINER-ENGINE).
// Only the endpoints the engine authorization plugin allows are used; the interface lets tests use a fake.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Engine is the set of engine operations of the tool (SSR-AGE-007 allowlist).
type Engine interface {
	Ping(ctx context.Context) error
	ImageInspect(ctx context.Context, id string) (*ImageInfo, error) // ErrNotFound when absent
	ImageLoad(ctx context.Context, tar io.Reader) error
	ContainerCreate(ctx context.Context, name string, body CreateBody) (string, error)
	ContainerList(ctx context.Context, labelFilters []string, all bool) ([]ContainerSummary, error)
	ContainerInspect(ctx context.Context, id string) (*Inspect, error)
	// ContainerAttach streams the multiplexed output into stdout and stderr; the channel
	// delivers one value (nil on a clean end of stream) when the stream ended.
	ContainerAttach(ctx context.Context, id string, stdout, stderr io.Writer) (<-chan error, error)
	ContainerStart(ctx context.Context, id string) error
	// ContainerWait blocks until the next exit of the container (call it before start).
	ContainerWait(ctx context.Context, id string) (int, error)
	ContainerKill(ctx context.Context, id string) error
	ContainerRemove(ctx context.Context, id string) error // force; absent is not an error
	// Events subscribes to the events of one container; the channel is closed when ctx ends.
	Events(ctx context.Context, id string) (<-chan Event, error)
}

var ErrNotFound = errors.New("not found")

var _ Engine = (*Client)(nil)

func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

func asAPI(err error, t **APIError) bool { return errors.As(err, t) }

// APIError is an engine answer with a non-success status.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("engine status %d: %s", e.Status, e.Message) }

// IsDenied reports a refusal by the authorization plugin (SFR-AGE-010 seen from the tool, exc-8).
func IsDenied(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Status == 403 || strings.Contains(strings.ToLower(ae.Message), "authorization denied")
}

type ImageInfo struct {
	ID     string `json:"Id"`
	Config struct {
		Env []string `json:"Env"`
	} `json:"Config"`
}

type ContainerSummary struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
	State  string            `json:"State"`
}

type Event struct {
	Type       string
	Action     string
	ID         string
	Attributes map[string]string
}

// Mount is a bind mount of the create body.
type Mount struct {
	Type     string `json:"Type"`
	Source   string `json:"Source,omitempty"`
	Target   string `json:"Target"`
	ReadOnly bool   `json:"ReadOnly"`
}

// HostConfig holds the fields of HostConfig the tool sets or verifies.
type HostConfig struct {
	Mounts         []Mount           `json:"Mounts,omitempty"`
	Tmpfs          map[string]string `json:"Tmpfs,omitempty"`
	NetworkMode    string            `json:"NetworkMode"`
	CapAdd         []string          `json:"CapAdd"`
	CapDrop        []string          `json:"CapDrop"`
	SecurityOpt    []string          `json:"SecurityOpt"`
	ReadonlyRootfs bool              `json:"ReadonlyRootfs"`
	Privileged     bool              `json:"Privileged"`
	AutoRemove     bool              `json:"AutoRemove"`
	Memory         int64             `json:"Memory"`
	NanoCpus       int64             `json:"NanoCpus"`
	PidsLimit      int64             `json:"PidsLimit"`
	OomKillDisable bool              `json:"OomKillDisable"`
	PidMode        string            `json:"PidMode"`
	IpcMode        string            `json:"IpcMode"`
	UTSMode        string            `json:"UTSMode"`
	UsernsMode     string            `json:"UsernsMode"`
	CgroupnsMode   string            `json:"CgroupnsMode"`
	CgroupParent   string            `json:"CgroupParent,omitempty"`
	Runtime        string            `json:"Runtime,omitempty"`
	GroupAdd       []string          `json:"GroupAdd,omitempty"`
	Binds          []string          `json:"Binds,omitempty"`
	Links          []string          `json:"Links,omitempty"`
	VolumesFrom    []string          `json:"VolumesFrom,omitempty"`
	Devices        []any             `json:"Devices,omitempty"`
	Sysctls        map[string]string `json:"Sysctls,omitempty"`
	PortBindings   map[string]any    `json:"PortBindings,omitempty"`
}

// CreateBody is the body of POST /containers/create.
type CreateBody struct {
	Image        string            `json:"Image"`
	User         string            `json:"User"`
	Env          []string          `json:"Env"`
	Cmd          []string          `json:"Cmd"`
	Entrypoint   []string          `json:"Entrypoint"` // empty, not absent: the image entrypoint is reset
	WorkingDir   string            `json:"WorkingDir"`
	Labels       map[string]string `json:"Labels"`
	Tty          bool              `json:"Tty"`
	OpenStdin    bool              `json:"OpenStdin"`
	AttachStdin  bool              `json:"AttachStdin"`
	AttachStdout bool              `json:"AttachStdout"`
	AttachStderr bool              `json:"AttachStderr"`
	HostConfig   HostConfig        `json:"HostConfig"`
}

// MountPoint is an entry of the top-level Mounts of an inspected container.
type MountPoint struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

// Inspect is the part of GET /containers/<id>/json the tool verifies.
type Inspect struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	Image string `json:"Image"`
	State struct {
		Running   bool `json:"Running"`
		ExitCode  int  `json:"ExitCode"`
		OOMKilled bool `json:"OOMKilled"`
	} `json:"State"`
	Config struct {
		User       string            `json:"User"`
		Env        []string          `json:"Env"`
		Cmd        []string          `json:"Cmd"`
		Entrypoint []string          `json:"Entrypoint"`
		WorkingDir string            `json:"WorkingDir"`
		Labels     map[string]string `json:"Labels"`
		Tty        bool              `json:"Tty"`
		OpenStdin  bool              `json:"OpenStdin"`
	} `json:"Config"`
	HostConfig HostConfig   `json:"HostConfig"`
	Mounts     []MountPoint `json:"Mounts"`
}
