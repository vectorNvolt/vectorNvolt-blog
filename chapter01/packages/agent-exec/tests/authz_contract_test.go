package tests

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-exec/internal/authz"
	"agent-exec/internal/kind"
	"agent-exec/internal/profile"
)

// Contract between the tool (kind.NewPlan, marshalled as engine.Client.ContainerCreate does)
// and the engine authorization plugin policy (authz.Policy): what the tool sends must be allowed.

const (
	contractLimits = "limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 64m }\n"
	contractImage  = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func contractProfile(t *testing.T, name, y string) *profile.Profile {
	t.Helper()
	p, err := profile.Parse([]byte(strings.ReplaceAll(y, "IMAGEID", contractImage)+contractLimits), name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// contractRoot is a fake filesystem root holding the workspace folders the binds point at.
func contractRoot(t *testing.T, execID string) *authz.Policy {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{
		"src", "build", ".venv", ".npm-env", ".conda-env", ".packages",
		".output/exec/" + execID + "/in", ".output/exec/" + execID + "/dl", ".output/exec/" + execID + "/out",
	} {
		if err := os.MkdirAll(filepath.Join(root, "workspace", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &authz.Policy{Root: root}
}

// createRequest returns the URI (as the client builds it) and the JSON body of a plan.
func createRequest(t *testing.T, pl *kind.Plan) (string, []byte) {
	t.Helper()
	raw, err := json.Marshal(pl.Body)
	if err != nil {
		t.Fatal(err)
	}
	return "/containers/create?" + url.Values{"name": {pl.Name}}.Encode(), raw
}

func TestToolBodiesAreAllowedByPlugin(t *testing.T) {
	const id = "ab12cd34"
	codeP := contractProfile(t, "python", "name: python\nkind: code\nimage_id: IMAGEID\nentry: agent\nbuild_dir: true\nenv_dirs: { .venv: ro }\nenv: { PATH: /workspace/.venv/bin:/usr/bin }\n")
	buildP := contractProfile(t, "c-make", "name: c-make\nkind: build\nimage_id: IMAGEID\nentry: fixed-args\nargv: [make]\nbuild_dir: true\nenv_dirs: { .npm-env: ro }\ntmp_exec: true\n")
	// Kind tool (USC-009 step 6): src/ ro, build/ ro (declared), own in/ ro, own out/ rw, network none.
	// The plugin needs no change for it: every bind is on its existing allowlist.
	toolP := contractProfile(t, "ngspice", "name: ngspice\nkind: tool\nimage_id: IMAGEID\nentry: fixed\n"+
		"argv: [ngspice, -b, -n, -r, \"{out}/sim.raw\", \"{in.netlist}\"]\nbuild_dir: true\n"+
		"inputs: { netlist: { path_under: [src/], glob: '*.cir' } }\ninput_policy: spice-safe\n")
	installP := contractProfile(t, "install-python", "name: install-python\nkind: install\nimage_id: IMAGEID\nentry: fixed-args\nfetch_argv: [pip, download]\nargv: [pip, install]\nenv_dirs: { .venv: rw, .npm-env: ro }\n")

	cases := []struct {
		name  string
		p     *profile.Profile
		phase string
	}{
		{"code run", codeP, kind.PhaseRun},
		{"build", buildP, kind.PhaseRun},
		{"tool", toolP, kind.PhaseRun},
		{"install fetch", installP, kind.PhaseFetch},
		{"install install", installP, kind.PhaseInstall},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pol := contractRoot(t, id)
			pl := kind.NewPlan(kind.Params{
				Profile: c.p, ExecID: id, Phase: c.phase, WorkspaceRoot: "/workspace",
				Argv: []string{"x"}, Pid: 4242, Labels: map[string]string{"agent-graph.run": "r1"},
			})
			uri, raw := createRequest(t, pl)
			if want := "ephemeral-" + id; c.phase == kind.PhaseFetch && !strings.HasSuffix(uri, want+"-fetch") || c.phase != kind.PhaseFetch && !strings.HasSuffix(uri, want) {
				t.Fatalf("unexpected create uri %s", uri)
			}
			if ok, msg := pol.Decide("POST", uri, raw); !ok {
				t.Fatalf("plugin denied the tool's create body: %s\n%s", msg, raw)
			}
			// The same body under a versioned path is decided the same way.
			if ok, msg := pol.Decide("POST", "/v1.43"+uri, raw); !ok {
				t.Fatalf("plugin denied the versioned request: %s", msg)
			}

			// A mutated body must be denied.
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			m["HostConfig"].(map[string]any)["Privileged"] = true
			bad, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if ok, _ := pol.Decide("POST", uri, bad); ok {
				t.Fatal("plugin allowed a privileged create body")
			}
		})
	}
}
