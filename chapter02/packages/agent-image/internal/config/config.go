// Package config holds every fixed value of create_agent_image.
package config

import "time"

// InterruptGrace (SFR-IMG-018): a Ctrl-C can end a console read with EOF/an error
// before the signal handler has cancelled the context; wait this long for the cancel.
const InterruptGrace = 500 * time.Millisecond

const (
	DockerExe = "docker"

	ImageName      = "agent-image"
	DefaultTag     = "latest"
	DockerfileName = "Dockerfile.agent"

	// The apt layer lives in a separate, content-addressed base image: agent-base:<hash of Dockerfile.base>.
	BaseDockerfileName = "Dockerfile.base"
	BaseImageRepo      = "agent-base"
	BaseHashLabel      = "vectornvolt.agent-base.hash"
	BaseHashLen        = 16 // hex characters of the sha256 used in the tag

	// TagPattern is the SFR-IMG-001 tag rule.
	TagPattern = `^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`
)

// BuildContextFiles are the build inputs read from the directory the binary is run from (the repo checkout inside the VM):
// Dockerfile.agent and every file it COPYs. They are sent to docker build as a tar on stdin.
var BuildContextFiles = []string{DockerfileName, "requirements.lock"}

// BinFiles are the prebuilt static Linux binaries Dockerfile.agent COPYs, read from the bin folder in
// that directory (slash paths, also the tar entry names). They are built in P8 and go into the tar with mode 0755.
var BinFiles = []string{"bin/agent-init", "bin/ephemeral-authz", "bin/run_ephemeral_container"}

// AgentGraphDir is the agent_graph Python package directory read from the same directory:
// a copy of packages/agent-graph/src/agent_graph. Dockerfile.agent COPYs it to /opt/agent_graph.
// Only its *.py files (any depth) and schemas/*.yml go into the build context; __pycache__ and
// *.egg-info directories are skipped, and every other file (e.g. *.pyc) is left out.
const AgentGraphDir = "agent_graph"

// AgentGraphRequired must be present in AgentGraphDir (package marker and `python -m` entry).
var AgentGraphRequired = []string{"__init__.py", "__main__.py"}

// ImageRef is the image reference for tag, e.g. agent-image:latest.
func ImageRef(tag string) string { return ImageName + ":" + tag }

// BaseImageRef is the base image reference for hash, e.g. agent-base:0123456789abcdef.
func BaseImageRef(hash string) string { return BaseImageRepo + ":" + hash }

// BaseArchiveName is the optional docker save archive of the base image in the current directory
// (a manual runbook step; the tool never writes it), e.g. agent-base-0123456789abcdef.tar.
func BaseArchiveName(hash string) string { return BaseImageRepo + "-" + hash + ".tar" }

// Config has no tunables: every value is fixed above.
type Config struct{}

// Load builds the configuration from getenv (os.Getenv in production).
func Load(getenv func(string) string) Config { return Config{} }
