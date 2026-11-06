// Package config holds every fixed value of create_agent_image.
package config

import "time"

// InterruptGrace (SFR-IMG-018): on Windows a Ctrl-C can end a console read with EOF/an error
// before the signal handler has cancelled the context; wait this long for the cancel.
const InterruptGrace = 500 * time.Millisecond

const (
	WSLExe     = "wsl.exe"
	DockerExe  = "docker"
	HostDistro = "agent-host"

	ImageName      = "agent-image"
	DefaultTag     = "latest"
	DockerfileName = "Dockerfile.agent"

	// TagPattern is the SFR-IMG-001 tag rule.
	TagPattern = `^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`
)

// BuildContextFiles are the build inputs read from the Windows directory the exe is run from:
// Dockerfile.agent and every file it COPYs. They are sent to docker build as a tar on stdin.
var BuildContextFiles = []string{DockerfileName, "requirements.lock"}

// ImageRef is the image reference for tag, e.g. agent-image:latest.
func ImageRef(tag string) string { return ImageName + ":" + tag }

// Config has no tunables: every value is fixed above.
type Config struct{}

// Load builds the configuration from getenv (os.Getenv in production).
func Load(getenv func(string) string) Config { return Config{} }
