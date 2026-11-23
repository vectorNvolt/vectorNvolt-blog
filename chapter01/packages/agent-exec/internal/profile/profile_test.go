package profile

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
)

var img = "sha256:" + strings.Repeat("a", 64)

const limitsOK = "limits: { mem: 512m, cpus: 1.5, pids: 100, timeout_s: 60, out_max: 64m }\n"

func TestImgConstant(t *testing.T) {
	if !imageIDRe.MatchString(img) {
		t.Fatalf("test constant is not a valid image id: %d chars", len(img))
	}
}

func TestParseValid(t *testing.T) {
	cases := map[string]string{
		"python": `
name: python
kind: code
image_id: ` + img + `
ref: exec-python:1
entry: agent
env_dirs: { .venv: ro }
env: { PATH: /workspace/.venv/bin:/usr/bin }
inputs: { data: { path_under: [.input/, .agent/, .output/exec/, src/], multiple: true } }
` + limitsOK,
		"c-make": `
name: c-make
kind: build
image_id: ` + img + `
entry: fixed-args
argv: [make]
build_dir: true
tmp_exec: true
` + limitsOK,
		"install-python": `
name: install-python
kind: install
image_id: ` + img + `
entry: fixed-args
fetch_argv: [pip, download, --only-binary=:all:, -d, "/workspace/.output/exec/<execId>/dl"]
argv: [install-into-venv, install, --no-index]
env_dirs: { .venv: rw }
` + limitsOK,
	}
	for dir, y := range cases {
		p, err := Parse([]byte(y), dir)
		if err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		if p.MemBytes != 512<<20 || p.NanoCPUs != 1_500_000_000 || p.Pids != 100 || p.TimeoutS != 60 || p.OutMaxBytes != 64<<20 {
			t.Errorf("%s: derived limits %+v", dir, p)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	base := func(kindLines string) string {
		return "name: p\nimage_id: " + img + "\n" + kindLines + limitsOK
	}
	code := "kind: code\nentry: agent\n"
	cases := map[string]struct{ yaml, want string }{
		"unknown field":      {base(code) + "mounts: [/]\n", "mounts"},
		"empty":              {"", "empty"},
		"bad image id":       {strings.Replace(base(code), img, "sha256:abc", 1), "image_id"},
		"tag instead of id":  {strings.Replace(base(code), img, "python:3", 1), "image_id"},
		"name differs":       {strings.Replace(base(code), "name: p", "name: q", 1), "differs"},
		"tool entry agent":   {base("kind: tool\nentry: agent\n"), "entry: fixed"},
		"unknown kind":       {base("kind: daemon\nentry: agent\n"), "kind"},
		"code with argv":     {base(code) + "argv: [sh]\n", "argv"},
		"build without argv": {base("kind: build\nentry: fixed-args\nbuild_dir: true\n"), "argv"},
		"build no build_dir": {base("kind: build\nentry: fixed-args\nargv: [make]\n"), "build_dir"},
		"install no fetch":   {base("kind: install\nentry: fixed-args\nargv: [pip]\nenv_dirs: { .venv: rw }\n"), "fetch_argv"},
		"install no rw dir":  {base("kind: install\nentry: fixed-args\nargv: [pip]\nfetch_argv: [pip]\nenv_dirs: { .venv: ro }\n"), "rw"},
		"code rw env dir":    {base(code) + "env_dirs: { .venv: rw }\n", "read-only"},
		"foreign env dir":    {base(code) + "env_dirs: { .ssh: ro }\n", "env_dirs"},
		"bad env mode":       {base(code) + "env_dirs: { .venv: wx }\n", "mode"},
		"env OUT":            {base(code) + "env: { OUT: /x }\n", "OUT"},
		"requires_build":     {base(code) + "requires_build: true\n", "build_dir"},
		"input from .tools":  {base(code) + "inputs: { d: { path_under: [.tools/] } }\n", "allowed"},
		"input escapes":      {base(code) + "inputs: { d: { path_under: [../etc/] } }\n", "relative"},
		"input without path": {base(code) + "inputs: { d: { glob: '*.csv' } }\n", "path_under"},
	}
	for name, c := range cases {
		_, err := Parse([]byte(c.yaml), "p")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want containing %q", name, err, c.want)
		}
	}
}

func TestLimitsCeilings(t *testing.T) {
	head := "name: p\nkind: code\nentry: agent\nimage_id: " + img + "\n"
	cases := map[string]string{
		"mem above":     "limits: { mem: 3g, cpus: 1, pids: 100, timeout_s: 60, out_max: 1m }",
		"cpus above":    "limits: { mem: 1g, cpus: 3, pids: 100, timeout_s: 60, out_max: 1m }",
		"pids above":    "limits: { mem: 1g, cpus: 1, pids: 513, timeout_s: 60, out_max: 1m }",
		"timeout above": "limits: { mem: 1g, cpus: 1, pids: 100, timeout_s: 3601, out_max: 1m }",
		"out above":     "limits: { mem: 1g, cpus: 1, pids: 100, timeout_s: 60, out_max: 2g }",
		"TBD":           "limits: { mem: TBD, cpus: TBD, pids: TBD, timeout_s: TBD, out_max: TBD }",
		"missing":       "limits: { mem: 1g }",
	}
	for name, l := range cases {
		if _, err := Parse([]byte(head+l+"\n"), "p"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := "limits: { mem: 2g, cpus: 2, pids: 512, timeout_s: 3600, out_max: 1g }"
	if _, err := Parse([]byte(head+ok+"\n"), "p"); err != nil {
		t.Errorf("ceilings themselves must pass: %v", err)
	}
}

func TestParseSize(t *testing.T) {
	good := map[string]int64{"512m": 512 << 20, "2g": 2 << 30, "64K": 64 << 10, "1048576": 1 << 20, "10mb": 10 << 20, "7b": 7}
	for in, want := range good {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "m", "0", "-1m", "1.5g", "abc", "99999999999999g"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) accepted", in)
		}
	}
}

func TestSubstitute(t *testing.T) {
	got := Substitute([]string{"-d", "/workspace/.output/exec/<execId>/dl", "$HOME"}, "e1")
	if got[1] != "/workspace/.output/exec/e1/dl" || got[2] != "$HOME" {
		t.Fatalf("%v", got)
	}
}

func mkTar(files ...[2]string) *bytes.Reader {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range files {
		tw.WriteHeader(&tar.Header{Name: f[0], Mode: 0o644, Size: int64(len(f[1]))})
		tw.Write([]byte(f[1]))
	}
	tw.Close()
	return bytes.NewReader(buf.Bytes())
}

func TestTarImageIDs(t *testing.T) {
	hex := strings.Repeat("c", 64)
	idx := "sha256:" + strings.Repeat("d", 64)
	manifest := func(config string) [2]string {
		return [2]string{"manifest.json", `[{"Config":"` + config + `"}]`}
	}
	index := func(digest string) [2]string {
		return [2]string{"index.json", `{"schemaVersion":2,"manifests":[{"digest":"` + digest + `"}]}`}
	}
	layer := [2]string{"layer.tar", "x"}

	// classic store: manifest.json only
	for _, cfg := range []string{hex + ".json", "blobs/sha256/" + hex} {
		ids, err := TarImageIDs(mkTar(layer, manifest(cfg)))
		if err != nil || len(ids) != 1 || ids[0] != "sha256:"+hex {
			t.Errorf("%s: %v %v", cfg, ids, err)
		}
	}
	// containerd store: index.json and manifest.json, either order
	for name, files := range map[string][][2]string{
		"index first":     {index(idx), manifest("blobs/sha256/" + hex), layer},
		"index after":     {layer, manifest("blobs/sha256/" + hex), index(idx)},
		"index in middle": {manifest("blobs/sha256/" + hex), index(idx), layer},
	} {
		ids, err := TarImageIDs(mkTar(files...))
		if err != nil || len(ids) != 2 || !contains(ids, "sha256:"+hex) || !contains(ids, idx) {
			t.Errorf("%s: %v %v", name, ids, err)
		}
		if ok, err := TarHasImageID(mkTar(files...), idx); err != nil || !ok {
			t.Errorf("%s: index digest not matched: %v %v", name, ok, err)
		}
		if ok, err := TarHasImageID(mkTar(files...), img); err != nil || ok {
			t.Errorf("%s: foreign ID matched: %v %v", name, ok, err)
		}
	}
	// index.json alone is enough
	if ids, err := TarImageIDs(mkTar(index(idx))); err != nil || len(ids) != 1 || ids[0] != idx {
		t.Errorf("index only: %v %v", ids, err)
	}
	// rejected
	if _, err := TarImageIDs(mkTar(layer, manifest("weird"))); err == nil {
		t.Error("unexpected config name accepted")
	}
	for _, bad := range []string{"sha256:abc", "sha512:" + strings.Repeat("d", 64), strings.Repeat("d", 64), "sha256:" + strings.Repeat("D", 64)} {
		if _, err := TarImageIDs(mkTar(manifest(hex+".json"), index(bad))); err == nil {
			t.Errorf("bad index digest %q accepted", bad)
		}
	}
	if _, err := TarImageIDs(mkTar([2]string{"index.json", "not json"})); err == nil {
		t.Error("unreadable index.json accepted")
	}
	if _, err := TarImageIDs(mkTar(layer, [2]string{"index.json", `{"manifests":[]}`})); err == nil {
		t.Error("tar without any ID accepted")
	}
	if _, err := TarImageIDs(bytes.NewReader(nil)); err == nil {
		t.Error("empty tar accepted")
	}
	if ok, err := TarHasImageID(bytes.NewReader(nil), idx); err == nil || ok {
		t.Error("TarHasImageID on empty tar did not error")
	}
}
