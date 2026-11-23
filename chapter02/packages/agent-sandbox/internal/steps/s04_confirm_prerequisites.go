// SFR-AGC-003/023 (USC-002 step 2, err-4/5/6): agents-net and agent-image:latest must exist; the Workspace-Agents root
// must exist, be a real directory (not a symlink) owned by agent-operator. Create, pull and repair nothing.
package steps

import (
	"context"
	"fmt"
	"os"

	"agent-sandbox/internal/config"
)

func ConfirmPrerequisites(ctx context.Context, env *Env) *Failure {
	var missing []string

	if ok, err := env.Docker.NetworkExists(ctx, config.NetworkName); err != nil {
		missing = append(missing, fmt.Sprintf("network %s: cannot check: %v", config.NetworkName, err))
	} else if !ok {
		missing = append(missing, fmt.Sprintf("network %s does not exist (created by create_agent_net)", config.NetworkName))
	}

	if ok, err := env.Docker.ImageExists(ctx, config.Image); err != nil {
		missing = append(missing, fmt.Sprintf("image %s: cannot check: %v", config.Image, err))
	} else if !ok {
		missing = append(missing, fmt.Sprintf("image %s is not available locally (neither built nor pulled by this tool)", config.Image))
	}

	if msg := checkRoot(env); msg != "" {
		missing = append(missing, msg)
	}

	if len(missing) > 0 {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateNotCreated,
			Message:   "shared prerequisites missing; nothing was created or pulled",
			Checks:    missing,
		}
	}
	return nil
}

// checkRoot returns the failed SFR-AGC-023 check of the Workspace-Agents root, or "" when it passes.
func checkRoot(env *Env) string {
	root := env.Config.WorkspaceRoot
	fi, err := env.FS.Lstat(root)
	switch {
	case os.IsNotExist(err):
		return fmt.Sprintf("workspace folder %s does not exist", root)
	case err != nil:
		return fmt.Sprintf("workspace folder %s cannot be inspected: %v", root, err)
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Sprintf("workspace folder %s is a symbolic link", root)
	case !fi.IsDir():
		return fmt.Sprintf("workspace folder %s is not a directory", root)
	}
	uid, _, err := env.FS.Owner(root)
	if err != nil {
		return fmt.Sprintf("owner of workspace folder %s cannot be read: %v", root, err)
	}
	if uid != config.WorkspaceUID {
		return fmt.Sprintf("workspace folder %s is owned by uid %d, want %d (agent-operator)", root, uid, config.WorkspaceUID)
	}
	return ""
}
