// SFR-HOST-013 (USC-001 step 3): read the tool builds from the folder the tool runs from and
// compute their reference SHA-256 on the host. Changes nothing.
package steps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-host/internal/config"
)

func ObtainToolHashes(_ context.Context, env *Env) *Failure {
	builds := map[string][]byte{}
	hashes := map[string]string{}
	var missing []string
	for _, name := range config.ToolNames {
		p := filepath.Join(env.Config.ToolsDir, name)
		b, err := os.ReadFile(p)
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s (%v)", p, err))
			continue
		}
		sum := sha256.Sum256(b)
		builds[name] = b
		hashes[name] = hex.EncodeToString(sum[:])
	}
	if len(missing) > 0 {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateNotCreated,
			Message:   "missing or unreadable tool build(s):\n" + strings.Join(missing, "\n"),
		}
	}
	env.ToolBuilds = builds
	env.ToolHashes = hashes
	return ReadEndpointsFile(env)
}
