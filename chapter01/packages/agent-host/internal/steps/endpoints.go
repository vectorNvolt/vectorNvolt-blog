// SFR-HOST-001 / SFR-HOST-011 (USC-001 steps 3 and 11): the model endpoints file endpoints.json.
// It is read from the folder the executable runs from (checked in step 3, before anything is created) and, once the
// tools and units are in place, sent as bytes over wsl.exe stdin to `refresh_endpoints`, which validates it,
// resolves the names and installs /etc/agent-net/endpoints.json (root:root 644). The file is never passed by path.
// The validator mirrors agent-net's ParseEndpoints (agent-host cannot import that internal package).
package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"agent-host/internal/config"
)

var hostLabelRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// validEndpoint accepts an IPv4 literal or a host name (labels of letters, digits and hyphens, not purely numeric).
func validEndpoint(s string) bool {
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Is4()
	}
	if s == "" || len(s) > 253 {
		return false
	}
	labels := strings.Split(s, ".")
	for _, l := range labels {
		if !hostLabelRe.MatchString(l) {
			return false
		}
	}
	last := labels[len(labels)-1]
	return strings.Trim(last, "0123456789") != ""
}

// ValidateEndpoints checks `{"endpoints": ["host", ...]}`: at most 64 KiB, no unknown keys, at least one entry,
// each a valid host name or IPv4 literal, no duplicates (case-insensitive).
func ValidateEndpoints(data []byte) error {
	if len(data) > config.EndpointsMaxBytes {
		return fmt.Errorf("larger than %d bytes", config.EndpointsMaxBytes)
	}
	var doc struct {
		Endpoints []string `json:"endpoints"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid JSON: unexpected data after the JSON object")
	}
	if len(doc.Endpoints) == 0 {
		return errors.New(`"endpoints" is missing or empty`)
	}
	seen := map[string]bool{}
	for _, e := range doc.Endpoints {
		if !validEndpoint(e) {
			return fmt.Errorf("%q is not a valid host name or IPv4 address", e)
		}
		k := strings.ToLower(e)
		if seen[k] {
			return fmt.Errorf("duplicate endpoint %q", e)
		}
		seen[k] = true
	}
	return nil
}

// ReadEndpointsFile reads and validates endpoints.json from the tools folder (USC-001 step 3). Changes nothing.
func ReadEndpointsFile(env *Env) *Failure {
	p := filepath.Join(env.Config.ToolsDir, config.EndpointsFileName)
	b, err := os.ReadFile(p)
	if err != nil {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated,
			Message: fmt.Sprintf("missing or unreadable endpoints file %s (%v)", p, err)}
	}
	if err := ValidateEndpoints(b); err != nil {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated,
			Message: fmt.Sprintf("invalid endpoints file %s: %v", p, err)}
	}
	env.EndpointsJSON = b
	return nil
}

// SeedEndpoints (USC-001 step 11, after the tools, units and docker are up): send the file bytes to
// `refresh_endpoints` on stdin as root, then check the installed file is root:root 644.
func SeedEndpoints(ctx context.Context, env *Env) *Failure {
	tool := config.ToolInstallDir + "/refresh_endpoints"
	if _, err := env.WSL.Exec(ctx, "root", env.EndpointsJSON, tool); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "seeding "+config.EndpointsInstallPath+" with refresh_endpoints failed", err)
	}
	out, err := env.WSL.Exec(ctx, "root", nil, "stat", "-c", "%U:%G %a", config.EndpointsInstallPath)
	if err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "checking "+config.EndpointsInstallPath+" failed", err)
	}
	if got := strings.TrimSpace(string(out)); got != "root:root 644" {
		return &Failure{ExitCode: ExitRuntime, ExitState: StatePartiallyConfigured,
			Message: fmt.Sprintf("%s owner/group/mode is %q, want \"root:root 644\"", config.EndpointsInstallPath, got)}
	}
	return nil
}
