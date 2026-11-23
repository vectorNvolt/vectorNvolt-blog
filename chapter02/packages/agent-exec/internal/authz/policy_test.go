package authz

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agent-exec/internal/limits"
)

type obj = map[string]any

// newRoot builds a fake filesystem root with the workspace folders of exec "abc".
func newRoot(t *testing.T) *Policy {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{
		"workspace/src", "workspace/build", "workspace/.venv", "workspace/.npm-env",
		"workspace/.conda-env", "workspace/.packages",
		"workspace/.output/exec/abc/in", "workspace/.output/exec/abc/dl", "workspace/.output/exec/abc/out",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &Policy{Root: root}
}

func bind(src string, ro bool) obj {
	return obj{"Type": "bind", "Source": src, "Target": src, "ReadOnly": ro,
		"BindOptions": obj{"Propagation": "rprivate"}}
}

func tmpfs(target string, size int64) obj {
	return obj{"Type": "tmpfs", "Target": target, "TmpfsOptions": obj{"SizeBytes": size}}
}

func tmpfsAll() []any {
	return []any{
		tmpfs("/work", limits.WorkTmpfsBytes),
		tmpfs("/tmp", limits.TmpTmpfsBytes),
		tmpfs("/home/run", limits.HomeTmpfsBytes),
	}
}

// body builds a create body the way the tool would send it.
func body(label, network string, mounts ...any) obj {
	return obj{
		"Image":  "sha256:0123",
		"User":   "1001:1001",
		"Labels": obj{"ephemeral.exec": label},
		"HostConfig": obj{
			"Privileged":     false,
			"CapDrop":        []string{"ALL"},
			"SecurityOpt":    []string{"no-new-privileges"},
			"ReadonlyRootfs": true,
			"NetworkMode":    network,
			"Memory":         limits.MaxMemoryBytes,
			"NanoCpus":       limits.MaxNanoCPUs,
			"PidsLimit":      limits.MaxPids,
			"AutoRemove":     true,
			"CgroupnsMode":   "host",
			"Mounts":         append(append([]any{}, mounts...), tmpfsAll()...),
		},
	}
}

func hc(b obj) obj { return b["HostConfig"].(obj) }

const ws = "/workspace"
const eo = "/workspace/.output/exec/abc"

func codeBody() obj {
	return body("abc", "none",
		bind(ws+"/src", true), bind(ws+"/build", true), bind(ws+"/.venv", true),
		bind(eo+"/in", true), bind(eo+"/out", false))
}

// tmpfsMapBody is codeBody with the scratch areas in HostConfig.Tmpfs (the shape the tool sends).
func tmpfsMapBody(tm obj) obj {
	b := codeBody()
	var binds []any
	for _, m := range hc(b)["Mounts"].([]any) {
		if m.(obj)["Type"] == "bind" {
			binds = append(binds, m)
		}
	}
	hc(b)["Mounts"] = binds
	hc(b)["Tmpfs"] = tm
	return b
}

func toolTmpfs() obj {
	return obj{
		"/work":     "rw,noexec,nosuid,nodev,size=536870912,mode=1777",
		"/tmp":      "rw,noexec,nosuid,nodev,size=536870912,mode=1777",
		"/home/run": "rw,noexec,nosuid,nodev,size=134217728,mode=1777",
	}
}

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const createURI = "/v1.43/containers/create?name=ephemeral-abc"

func TestCreateAllowed(t *testing.T) {
	p := newRoot(t)
	cases := []struct {
		name, uri string
		b         obj
	}{
		{"code", createURI, codeBody()},
		{"build", createURI, body("abc", "none",
			bind(ws+"/src", true), bind(ws+"/build", false),
			bind(eo+"/in", true), bind(eo+"/out", false))},
		{"install-fetch-host", "/containers/create?name=ephemeral-abc-fetch", body("abc", "host",
			bind(ws+"/src", true), bind(ws+"/.packages", true),
			bind(eo+"/dl", false), bind(eo+"/out", false))},
		{"install-install", createURI, body("abc", "none",
			bind(ws+"/src", true), bind(ws+"/.packages", true), bind(ws+"/.venv", false),
			bind(eo+"/dl", true), bind(eo+"/out", false))},
		{"tmpfs-map-as-tool-sends", createURI, tmpfsMapBody(toolTmpfs())},
		{"tmpfs-map-suffixes", createURI, tmpfsMapBody(obj{"/work": "size=512m", "/tmp": "exec,size=512M", "/home/run": "size=131072k"})},
		{"tmpfs-map-subset", createURI, tmpfsMapBody(obj{"/tmp": "rw,size=1048576"})},
		{"tmpfs-and-other-mounts", createURI, func() obj {
			b := tmpfsMapBody(obj{"/work": "rw,size=1m"})
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), tmpfs("/tmp", limits.TmpTmpfsBytes))
			return b
		}()},
		{"no-new-privileges-true", createURI, func() obj {
			b := codeBody()
			hc(b)["SecurityOpt"] = []string{"no-new-privileges:true"}
			return b
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ok, msg := p.Decide("POST", c.uri, marshal(t, c.b)); !ok {
				t.Fatalf("denied: %s", msg)
			}
		})
	}
}

func TestCreateDenied(t *testing.T) {
	root := newRoot(t)
	mut := func(f func(b obj)) obj { b := codeBody(); f(b); return b }
	cases := []struct {
		name string
		b    obj
		raw  []byte
		uri  string
		pol  *Policy
	}{
		{name: "privileged", b: mut(func(b obj) { hc(b)["Privileged"] = true })},
		{name: "user-root", b: mut(func(b obj) { b["User"] = "0" })},
		{name: "user-1001-only", b: mut(func(b obj) { b["User"] = "1001" })},
		{name: "group-add", b: mut(func(b obj) { hc(b)["GroupAdd"] = []string{"docker"} })},
		{name: "cap-add", b: mut(func(b obj) { hc(b)["CapAdd"] = []string{"SYS_ADMIN"} })},
		{name: "no-cap-drop", b: mut(func(b obj) { delete(hc(b), "CapDrop") })},
		{name: "seccomp-unconfined", b: mut(func(b obj) {
			hc(b)["SecurityOpt"] = []string{"no-new-privileges", "seccomp=unconfined"}
		})},
		{name: "no-nnp", b: mut(func(b obj) { hc(b)["SecurityOpt"] = []string{} })},
		{name: "writable-rootfs", b: mut(func(b obj) { hc(b)["ReadonlyRootfs"] = false })},
		{name: "net-container", b: mut(func(b obj) { hc(b)["NetworkMode"] = "container:x" })},
		{name: "net-bridge", b: mut(func(b obj) { hc(b)["NetworkMode"] = "bridge" })},
		{name: "pid-host", b: mut(func(b obj) { hc(b)["PidMode"] = "host" })},
		{name: "ipc-host", b: mut(func(b obj) { hc(b)["IpcMode"] = "host" })},
		{name: "cgroupns-private", b: mut(func(b obj) { hc(b)["CgroupnsMode"] = "private" })},
		{name: "cgroupns-empty", b: mut(func(b obj) { hc(b)["CgroupnsMode"] = "" })},
		{name: "cgroupns-absent", b: mut(func(b obj) { delete(hc(b), "CgroupnsMode") })},
		{name: "cgroupns-container", b: mut(func(b obj) { hc(b)["CgroupnsMode"] = "container:x" })},
		{name: "cgroup-parent", b: mut(func(b obj) { hc(b)["CgroupParent"] = "/x" })},
		{name: "bind-root", b: mut(func(b obj) {
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), bind("/", false))
		})},
		{name: "bind-docker-sock", b: mut(func(b obj) {
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), bind("/var/run/docker.sock", false))
		})},
		{name: "bind-other-exec-dir", b: mut(func(b obj) {
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), bind("/workspace/.output/exec/zzz/out", false))
		})},
		{name: "bind-dotdot", b: mut(func(b obj) {
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), bind("/workspace/src/../.agent", true))
		})},
		{name: "bind-src-rw", b: mut(func(b obj) {
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), bind(ws+"/.packages", false))
		})},
		{name: "bind-target-differs", b: mut(func(b obj) {
			m := bind(ws+"/.packages", true)
			m["Target"] = "/etc"
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), m)
		})},
		{name: "bind-propagation", b: mut(func(b obj) {
			m := bind(ws+"/.packages", true)
			m["BindOptions"] = obj{"Propagation": "rshared"}
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), m)
		})},
		{name: "volume-mount", b: mut(func(b obj) {
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), obj{"Type": "volume", "Source": "v", "Target": "/v"})
		})},
		{name: "binds-field", b: mut(func(b obj) { hc(b)["Binds"] = []string{"/workspace/src:/workspace/src:ro"} })},
		{name: "tmpfs-map", b: mut(func(b obj) { hc(b)["Tmpfs"] = obj{"/x": ""} })},
		{name: "tmpfs-map-missing-size", b: tmpfsMapBody(obj{"/tmp": "rw,noexec,nosuid,nodev,mode=1777"})},
		{name: "tmpfs-map-over-ceiling", b: tmpfsMapBody(obj{"/work": "rw,size=536870913"})},
		{name: "tmpfs-map-over-ceiling-suffix", b: tmpfsMapBody(obj{"/home/run": "size=1g"})},
		{name: "tmpfs-map-zero-size", b: tmpfsMapBody(obj{"/tmp": "size=0"})},
		{name: "tmpfs-map-target-etc", b: tmpfsMapBody(obj{"/etc": "rw,size=1m"})},
		{name: "tmpfs-map-unknown-option", b: tmpfsMapBody(obj{"/tmp": "rw,size=1m,uid=0"})},
		{name: "tmpfs-map-empty-option", b: tmpfsMapBody(obj{"/tmp": "rw,size=1m,"})},
		{name: "tmpfs-map-bad-size", b: tmpfsMapBody(obj{"/tmp": "size=12x"})},
		{name: "tmpfs-map-size-twice", b: tmpfsMapBody(obj{"/tmp": "size=1m,size=2m"})},
		{name: "tmpfs-map-also-in-mounts", b: func() obj {
			b := tmpfsMapBody(obj{"/work": "rw,size=1m"})
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), tmpfs("/work", limits.WorkTmpfsBytes))
			return b
		}()},
		{name: "tmpfs-no-size", b: mut(func(b obj) {
			hc(b)["Mounts"] = append(hc(b)["Mounts"].([]any), obj{"Type": "tmpfs", "Target": "/x"})
		})},
		{name: "tmpfs-over-ceiling", b: mut(func(b obj) {
			hc(b)["Mounts"] = []any{tmpfs("/work", limits.WorkTmpfsBytes+1)}
		})},
		{name: "id-label-mismatch", b: mut(func(b obj) { b["Labels"] = obj{"ephemeral.exec": "zzz"} })},
		{name: "no-label", b: mut(func(b obj) { b["Labels"] = obj{} })},
		{name: "bad-name", b: codeBody(), uri: "/containers/create?name=evil"},
		{name: "no-name", b: codeBody(), uri: "/containers/create"},
		{name: "missing-body", raw: []byte{}},
		{name: "null-body", raw: []byte("null")},
		{name: "garbage-body", raw: []byte("{not json")},
		{name: "trailing-data", raw: append(marshal(t, codeBody()), []byte(`{}`)...)},
		{name: "memory-over", b: mut(func(b obj) { hc(b)["Memory"] = limits.MaxMemoryBytes + 1 })},
		{name: "memory-unset", b: mut(func(b obj) { delete(hc(b), "Memory") })},
		{name: "nanocpus-over", b: mut(func(b obj) { hc(b)["NanoCpus"] = limits.MaxNanoCPUs + 1 })},
		{name: "pids-unlimited", b: mut(func(b obj) { hc(b)["PidsLimit"] = -1 })},
		{name: "pids-over", b: mut(func(b obj) { hc(b)["PidsLimit"] = limits.MaxPids + 1 })},
		{name: "oom-kill-disable", b: mut(func(b obj) { hc(b)["OomKillDisable"] = true })},
		{name: "autoremove-false", b: mut(func(b obj) { hc(b)["AutoRemove"] = false })},
		{name: "devices", b: mut(func(b obj) { hc(b)["Devices"] = []obj{{"PathOnHost": "/dev/kmsg"}} })},
		{name: "sysctls", b: mut(func(b obj) { hc(b)["Sysctls"] = obj{"a": "b"} })},
		{name: "port-bindings", b: mut(func(b obj) { hc(b)["PortBindings"] = obj{"80/tcp": []obj{{}}} })},
		{name: "volumes-from", b: mut(func(b obj) { hc(b)["VolumesFrom"] = []string{"x"} })},
		{name: "anonymous-volumes", b: mut(func(b obj) { b["Volumes"] = obj{"/v": obj{}} })},
		{name: "runtime", b: mut(func(b obj) { hc(b)["Runtime"] = "runsc" })},
		{name: "no-hostconfig", b: mut(func(b obj) { delete(b, "HostConfig") })},
		{name: "bind-symlink-component", b: codeBody(), pol: func() *Policy {
			p := newRoot(t)
			// build -> src: the agent swapped a workspace folder for a symlink.
			bp := filepath.Join(p.Root, "workspace/build")
			if err := os.Remove(bp); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(p.Root, "workspace/src"), bp); err != nil {
				t.Fatal(err)
			}
			return p
		}()},
		{name: "bind-source-missing", b: codeBody(), pol: &Policy{Root: t.TempDir()}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := root
			if c.pol != nil {
				p = c.pol
			}
			raw := c.raw
			if c.b != nil {
				raw = marshal(t, c.b)
			}
			uri := c.uri
			if uri == "" {
				uri = createURI
			}
			if ok, msg := p.Decide("POST", uri, raw); ok {
				t.Fatalf("allowed, want deny (%s)", msg)
			}
		})
	}
}

func TestEndpoints(t *testing.T) {
	p := newRoot(t)
	cases := []struct {
		method, uri string
		body        string
		want        bool
	}{
		{"GET", "/_ping", "", true},
		{"HEAD", "/v1.43/_ping", "", true},
		{"GET", "/v1.43/version", "", true},
		{"GET", "/info", "", true},
		{"GET", "/v1.43/images/json?all=1", "", true},
		{"GET", "/images/sha256:abc/json", "", true},
		{"POST", "/images/load?quiet=1", "", true},
		{"GET", "/containers/json?all=1", "", true},
		{"GET", "/containers/abc/json", "", true},
		{"POST", "/v1.43/containers/abc/start", "", true},
		{"POST", "/containers/abc/attach?stream=1&stdout=1", "", true},
		{"GET", "/containers/abc/logs?stdout=1", "", true},
		{"POST", "/containers/abc/wait", "", true},
		{"POST", "/containers/abc/kill?signal=KILL", "", true},
		{"DELETE", "/containers/abc?force=1", "", true},
		{"GET", "/events?filters=x", "", true},

		{"POST", "/containers/abc/start", "{}", false},
		{"POST", "/containers/abc/exec", "{}", false},
		{"POST", "/v1.43/containers/abc/exec", "{}", false},
		{"POST", "/exec/abc/start", "{}", false},
		{"GET", "/containers/abc/archive?path=/", "", false},
		{"PUT", "/containers/abc/archive?path=/", "x", false},
		{"GET", "/containers/abc/export", "", false},
		{"POST", "/commit?container=abc", "", false},
		{"POST", "/containers/abc/update", "{}", false},
		{"POST", "/build", "", false},
		{"POST", "/images/create?fromImage=alpine", "", false},
		{"POST", "/images/alpine/push", "", false},
		{"POST", "/images/alpine/tag?repo=x", "", false},
		{"GET", "/networks", "", false},
		{"POST", "/networks/create", "{}", false},
		{"GET", "/volumes", "", false},
		{"GET", "/plugins", "", false},
		{"GET", "/swarm", "", false},
		{"POST", "/session", "", false},
		{"POST", "/grpc", "", false},
		{"GET", "/containers/abc/attach/ws", "", false},
		{"POST", "/containers/json", "", false},
		{"GET", "/version/../exec", "", false},
		{"GET", "/containers//json", "", false},
		{"GET", "/%63ontainers/json", "", false},
		{"GET", "/v1.43", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		ok, msg := p.Decide(c.method, c.uri, []byte(c.body))
		if ok != c.want {
			t.Errorf("%s %s body=%q: allow=%v (%s), want %v", c.method, c.uri, c.body, ok, msg, c.want)
		}
	}
}

func TestDecideNeverPanics(t *testing.T) {
	p := newRoot(t)
	for _, b := range []string{"[]", `"x"`, "1", `{"HostConfig":[]}`, `{"HostConfig":{"Mounts":[null]}}`} {
		if ok, _ := p.Decide("POST", createURI, []byte(b)); ok {
			t.Errorf("body %s allowed", b)
		}
	}
}

func TestHandlerProtocol(t *testing.T) {
	Logger = log.New(io.Discard, "", 0)
	p := newRoot(t)
	srv := httptest.NewServer(NewHandler(p))
	defer srv.Close()

	post := func(path string, v any) (*http.Response, []byte) {
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(marshal(t, v)))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}

	resp, b := post("/Plugin.Activate", obj{})
	if ct := resp.Header.Get("Content-Type"); ct != pluginContentType {
		t.Errorf("content type %q", ct)
	}
	var act struct{ Implements []string }
	if err := json.Unmarshal(b, &act); err != nil || len(act.Implements) != 1 || act.Implements[0] != "authz" {
		t.Errorf("activate: %s %v", b, err)
	}

	decide := func(method, uri string, rb []byte) Response {
		_, b := post("/AuthZPlugin.AuthZReq", Request{RequestMethod: method, RequestURI: uri, RequestBody: rb})
		var out Response
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("response %s: %v", b, err)
		}
		return out
	}
	if r := decide("GET", "/v1.43/_ping", nil); !r.Allow {
		t.Errorf("ping denied: %s", r.Msg)
	}
	if r := decide("POST", createURI, marshal(t, codeBody())); !r.Allow {
		t.Errorf("create denied: %s", r.Msg)
	}
	hcb := codeBody()
	hc(hcb)["Privileged"] = true
	if r := decide("POST", createURI, marshal(t, hcb)); r.Allow || r.Msg == "" {
		t.Errorf("privileged create: %+v", r)
	}
	if r := decide("POST", "/containers/abc/exec", []byte("{}")); r.Allow {
		t.Errorf("exec allowed")
	}

	// Unparsable plugin request is denied.
	r2, err := http.Post(srv.URL+"/AuthZPlugin.AuthZReq", "application/json", bytes.NewReader([]byte("{")))
	if err != nil {
		t.Fatal(err)
	}
	var out Response
	_ = json.NewDecoder(r2.Body).Decode(&out)
	r2.Body.Close()
	if out.Allow {
		t.Errorf("unparsable request allowed")
	}

	// AuthZRes allows.
	_, b = post("/AuthZPlugin.AuthZRes", Request{RequestMethod: "GET", RequestURI: "/info", ResponseStatusCode: 200})
	if err := json.Unmarshal(b, &out); err != nil || !out.Allow {
		t.Errorf("AuthZRes: %s %v", b, err)
	}

	// GET is not part of the protocol.
	g, err := http.Get(srv.URL + "/AuthZPlugin.AuthZReq")
	if err != nil {
		t.Fatal(err)
	}
	g.Body.Close()
	if g.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status %d", g.StatusCode)
	}
}
