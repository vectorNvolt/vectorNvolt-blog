// SFR-HOST-002 (USC-001 step 2): confirm no distro named agent-host is registered.
package steps

import (
	"context"
	"fmt"

	"agent-host/internal/config"
	"agent-host/internal/services"
)

// ConfirmNoDistro checks that no WSL distro named agent-host is registered and that WSL meets the minimum version requirement. It also verifies the Startup folder if keep-alive is enabled.
// @param ctx the context for handling cancellation and timeouts,
// @param env the environment containing WSL and configuration
// @return a Failure if any prerequisite is not met, or nil if all checks pass
func ConfirmNoDistro(ctx context.Context, env *Env) *Failure {
	distros, err := env.WSL.List(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "WSL is unavailable (wsl.exe -l -v failed)", err)
	}
	if name, v, found := services.FindDistro(distros, config.DistroName); found {
		ver := "unknown"
		if v != services.UnknownWSLVersion {
			ver = fmt.Sprintf("%d", v)
		}
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateNotCreated,
			Message:   fmt.Sprintf("a distro named %s is already registered (listed as %q, WSL version %s); it was left untouched", config.DistroName, name, ver),
		}
	}
	// SFR-HOST-004: WSL must be recent enough to import the distro; checked before anything is downloaded.
	required := fmt.Sprintf("WSL %s or newer is required; run `wsl --update`", config.MinWSLVersion)
	v, err := env.WSL.Version(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, required+" (wsl.exe --version is unsupported or unreadable)", err)
	}
	if !services.VersionAtLeast(v, config.MinWSLVersion) {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated, Message: fmt.Sprintf("%s (found WSL %s)", required, v)}
	}
	// Keep-alive (step 11) needs the Startup folder; fail before anything is changed.
	if env.Config.KeepAlive {
		if _, err := KeepAlivePath(env.Config.AppData); err != nil {
			return fail(ExitPrerequisite, StateNotCreated, "keep-alive", err)
		}
	}
	return nil
}
