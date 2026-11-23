package wsread

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newWS(t *testing.T) (string, *Reader) {
	t.Helper()
	ws := t.TempDir()
	r, err := New(ws)
	if err != nil {
		t.Fatal(err)
	}
	return ws, r
}

func TestValidID(t *testing.T) {
	good := []string{"rcd1", "a", "A.b-c_d", "0", strings.Repeat("a", 64)}
	bad := []string{"", ".", "..", ".hidden", "-x", "_x", "a/b", "a\\b", "a b", "a\x00b", strings.Repeat("a", 65), "é"}
	for _, id := range good {
		if !ValidID(id) {
			t.Errorf("ValidID(%q) = false", id)
		}
	}
	for _, id := range bad {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true", id)
		}
	}
}

func TestCleanRel(t *testing.T) {
	ok := map[string]string{
		"result.json":   "result.json",
		"out/sim.raw":   "out/sim.raw",
		"out//sim.raw":  "out/sim.raw",
		"./out/sim.raw": "out/sim.raw",
		"in/netlist/":   "in/netlist",
	}
	for in, want := range ok {
		got, err := CleanRel(in)
		if err != nil || got != want {
			t.Errorf("CleanRel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"", ".", "/", "/etc/passwd", "..", "../x", "a/../b", "a/..", "out/../../x", "a\\b", "a\x00b", "./"}
	for _, in := range bad {
		if _, err := CleanRel(in); !errors.Is(err, ErrBadRequest) {
			t.Errorf("CleanRel(%q) err = %v; want ErrBadRequest", in, err)
		}
	}
}

func TestOpenFileOK(t *testing.T) {
	ws, r := newWS(t)
	write(t, filepath.Join(ws, ".output/exec/rcd1/out/sim.log"), "hello")
	f, st, err := r.OpenFile("rcd1", "out/sim.log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	if string(b) != "hello" || st.Size() != 5 {
		t.Fatalf("got %q size %d", b, st.Size())
	}
}

func TestOpenFileBadInput(t *testing.T) {
	ws, r := newWS(t)
	write(t, filepath.Join(ws, ".output/exec/rcd1/result.json"), "{}")
	for _, id := range []string{"", "..", "a/b", ".x"} {
		if _, _, err := r.OpenFile(id, "result.json"); !errors.Is(err, ErrBadRequest) {
			t.Errorf("id %q: err = %v; want ErrBadRequest", id, err)
		}
	}
	for _, rel := range []string{"", "../rcd1/result.json", "/etc/passwd", "a\\b", "x\x00y", ".."} {
		if _, _, err := r.OpenFile("rcd1", rel); !errors.Is(err, ErrBadRequest) {
			t.Errorf("rel %q: err = %v; want ErrBadRequest", rel, err)
		}
	}
}

func TestOpenFileNotFound(t *testing.T) {
	ws, r := newWS(t)
	if _, _, err := r.OpenFile("rcd1", "result.json"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no .output: err = %v", err)
	}
	write(t, filepath.Join(ws, ".output/exec/rcd1/result.json"), "{}")
	if _, _, err := r.OpenFile("rcd2", "result.json"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: err = %v", err)
	}
	if _, _, err := r.OpenFile("rcd1", "nope.txt"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing file: err = %v", err)
	}
}

func TestOpenFileDirectoryAndFIFO(t *testing.T) {
	ws, r := newWS(t)
	dir := filepath.Join(ws, ".output/exec/rcd1")
	write(t, filepath.Join(dir, "out/sim.raw"), "x")
	if _, _, err := r.OpenFile("rcd1", "out"); !errors.Is(err, ErrNotFound) {
		t.Errorf("directory: err = %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := r.OpenFile("rcd1", "pipe")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("fifo: err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fifo open blocked")
	}
}

func TestOpenFileSymlinkEscape(t *testing.T) {
	ws, r := newWS(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "secret"), "secret")
	write(t, filepath.Join(ws, "other/file"), "other")
	dir := filepath.Join(ws, ".output/exec/rcd1")
	write(t, filepath.Join(dir, "out/ok"), "ok")
	links := map[string]string{
		"filelink":      filepath.Join(outside, "secret"),
		"dirlink":       outside,
		"out/relescape": "../../../../other/file",
		"wsfile":        filepath.Join(ws, "other/file"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"filelink", "dirlink/secret", "out/relescape", "wsfile"} {
		if _, _, err := r.OpenFile("rcd1", rel); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v; want ErrNotFound", rel, err)
		}
	}
}

func TestOpenFileSymlinkInsideSameExecRefused(t *testing.T) {
	ws, r := newWS(t)
	dir := filepath.Join(ws, ".output/exec/rcd1")
	write(t, filepath.Join(dir, "out/real"), "r")
	if err := os.Symlink("real", filepath.Join(dir, "out/alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("out", filepath.Join(dir, "outalias")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"out/alias", "outalias/real"} {
		if _, _, err := r.OpenFile("rcd1", rel); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v; want ErrNotFound", rel, err)
		}
	}
	if _, _, err := r.OpenFile("rcd1", "out/real"); err != nil {
		t.Errorf("real file: %v", err)
	}
}

func TestOpenFileIntermediateDirSymlink(t *testing.T) {
	ws, r := newWS(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "sim.raw"), "SECRET")
	dir := filepath.Join(ws, ".output/exec/rcd1")
	write(t, filepath.Join(dir, "result.json"), "{}")
	if err := os.Symlink(outside, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenFile("rcd1", "out/sim.raw"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v; want ErrNotFound", err)
	}
	if got := r.ListExecs(); len(got) != 1 || len(got[0].Outputs) != 0 {
		t.Errorf("symlinked out dir listed: %+v", got)
	}
}

func TestOutputDirSymlinkedRejected(t *testing.T) {
	ws, r := newWS(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "exec/rcd1/result.json"), "{}")
	if err := os.Symlink(outside, filepath.Join(ws, ".output")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenFile("rcd1", "result.json"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	if got := r.ListExecs(); len(got) != 0 {
		t.Errorf("listed through symlinked .output: %+v", got)
	}
}

// TestSymlinkSwapRace swaps <exec>/out between a real dir and a symlink to an outside dir while
// reading through it. A read may succeed (INSIDE) or fail, but must never return the outside file.
func TestSymlinkSwapRace(t *testing.T) {
	ws, r := newWS(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "sim.raw"), "SECRET")
	dir := filepath.Join(ws, ".output/exec/rcd1")
	write(t, filepath.Join(dir, "realout/sim.raw"), "INSIDE")
	if err := os.Symlink(outside, filepath.Join(dir, "linkout")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.Rename(filepath.Join(dir, "realout"), out); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	swapDone := make(chan struct{})
	go func() {
		defer close(swapDone)
		// out is a real dir; "spare" is the other variant. Each step renames out away and spare in
		// (rename of a symlink over/away is atomic per call; the gap makes out briefly missing).
		spare := filepath.Join(dir, "linkout")
		hold := filepath.Join(dir, "hold")
		for {
			select {
			case <-stop:
				return
			default:
			}
			if os.Rename(out, hold) != nil {
				continue
			}
			_ = os.Rename(spare, out) // out is now the symlink
			_ = os.Rename(out, spare)
			_ = os.Rename(hold, out) // out is the real dir again
		}
	}()

	deadline := time.Now().Add(300 * time.Millisecond)
	reads := 0
	for time.Now().Before(deadline) {
		f, _, err := r.OpenFile("rcd1", "out/sim.raw")
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("unexpected error: %v", err)
			}
			continue
		}
		b, _ := io.ReadAll(f)
		f.Close()
		reads++
		if string(b) != "INSIDE" {
			close(stop)
			<-swapDone
			t.Fatalf("read %q through swapped symlink", b)
		}
	}
	close(stop)
	<-swapDone
	t.Logf("successful reads: %d", reads)
}

func TestOpenFileSiblingExecViaSymlinkRejected(t *testing.T) {
	ws, r := newWS(t)
	write(t, filepath.Join(ws, ".output/exec/rcd2/result.json"), "{}")
	dir := filepath.Join(ws, ".output/exec/rcd1")
	write(t, filepath.Join(dir, "x"), "x")
	if err := os.Symlink("../rcd2/result.json", filepath.Join(dir, "sib")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenFile("rcd1", "sib"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestExecDirSymlinkedRejected(t *testing.T) {
	ws, r := newWS(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "result.json"), "{}")
	if err := os.MkdirAll(filepath.Join(ws, ".output/exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, ".output/exec/evil")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenFile("evil", "result.json"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	if got := r.ListExecs(); len(got) != 0 {
		t.Errorf("symlinked exec dir listed: %+v", got)
	}
}

func TestExecBaseSymlinkedRejected(t *testing.T) {
	ws, r := newWS(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "rcd1/result.json"), "{}")
	if err := os.MkdirAll(filepath.Join(ws, ".output"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, ".output/exec")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenFile("rcd1", "result.json"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	if got := r.ListExecs(); len(got) != 0 {
		t.Errorf("listed through symlinked base: %+v", got)
	}
}

func TestWorkspaceRootIsSymlink(t *testing.T) {
	real := t.TempDir()
	write(t, filepath.Join(real, ".output/exec/rcd1/result.json"), "{}")
	link := filepath.Join(t.TempDir(), "ws")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	r, err := New(link)
	if err != nil {
		t.Fatal(err)
	}
	f, _, err := r.OpenFile("rcd1", "result.json")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestSizeCap(t *testing.T) {
	ws, r := newWS(t)
	p := filepath.Join(ws, ".output/exec/rcd1/out/big.raw")
	write(t, p, "")
	if err := os.Truncate(p, MaxFileSize+1); err != nil {
		t.Skipf("truncate: %v", err)
	}
	if _, _, err := r.OpenFile("rcd1", "out/big.raw"); !errors.Is(err, ErrTooBig) {
		t.Errorf("err = %v; want ErrTooBig", err)
	}
	if err := os.Truncate(p, MaxFileSize); err != nil {
		t.Fatal(err)
	}
	f, _, err := r.OpenFile("rcd1", "out/big.raw")
	if err != nil {
		t.Fatalf("exactly at cap: %v", err)
	}
	f.Close()
}

func TestNewRejectsNonDir(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing dir accepted")
	}
	p := filepath.Join(t.TempDir(), "f")
	write(t, p, "x")
	if _, err := New(p); err == nil {
		t.Error("file accepted")
	}
}

func TestListExecs(t *testing.T) {
	ws, r := newWS(t)
	if got := r.ListExecs(); got == nil || len(got) != 0 {
		t.Fatalf("missing .output/exec: %v", got)
	}
	base := filepath.Join(ws, ".output/exec")
	write(t, filepath.Join(base, "old/result.json"), `{"exit":0}`)
	write(t, filepath.Join(base, "old/meta.json"), `{"tool":"ngspice"}`)
	write(t, filepath.Join(base, "old/in/netlist"), "* net")
	write(t, filepath.Join(base, "old/out/sim.raw"), "raw")
	write(t, filepath.Join(base, "old/out/sim.log"), "log")
	write(t, filepath.Join(base, "bad/result.json"), `{not json`)
	write(t, filepath.Join(base, "bad/meta.json"), `[1,2]`)
	write(t, filepath.Join(base, "stray-file"), "x")
	write(t, filepath.Join(base, ".hidden/result.json"), "{}")
	if err := os.Symlink("../../../..", filepath.Join(base, "old/out/linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "old/out/subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(base, "old"), t0, t0); err != nil {
		t.Fatal(err)
	}
	t1 := time.Now()
	if err := os.Chtimes(filepath.Join(base, "bad"), t1, t1); err != nil {
		t.Fatal(err)
	}

	got := r.ListExecs()
	if len(got) != 2 {
		t.Fatalf("len = %d: %+v", len(got), got)
	}
	if got[0].ID != "bad" || got[1].ID != "old" {
		t.Fatalf("order: %s, %s", got[0].ID, got[1].ID)
	}
	b, _ := json.Marshal(got[0])
	if !strings.Contains(string(b), `"result":null`) || !strings.Contains(string(b), `"meta":null`) {
		t.Errorf("malformed json not null: %s", b)
	}
	if got[0].Inputs == nil || got[0].Outputs == nil {
		t.Errorf("inputs/outputs must be [] not nil")
	}
	old := got[1]
	if string(old.Result) != `{"exit":0}` || string(old.Meta) != `{"tool":"ngspice"}` {
		t.Errorf("result/meta: %s %s", old.Result, old.Meta)
	}
	if strings.Join(old.Inputs, ",") != "netlist" {
		t.Errorf("inputs = %v", old.Inputs)
	}
	if strings.Join(old.Outputs, ",") != "sim.log,sim.raw" {
		t.Errorf("outputs = %v", old.Outputs)
	}
	if _, err := time.Parse(time.RFC3339, old.MTime); err != nil {
		t.Errorf("mtime %q: %v", old.MTime, err)
	}
}

func TestFingerprintChanges(t *testing.T) {
	ws, r := newWS(t)
	base := filepath.Join(ws, ".output/exec/rcd1")
	f0 := r.Fingerprint()
	write(t, filepath.Join(base, "meta.json"), "{}")
	f1 := r.Fingerprint()
	if f0 == f1 {
		t.Error("new exec dir did not change fingerprint")
	}
	if r.Fingerprint() != f1 {
		t.Error("fingerprint not stable")
	}
	write(t, filepath.Join(base, "result.json"), "{}")
	f2 := r.Fingerprint()
	if f2 == f1 {
		t.Error("result.json did not change fingerprint")
	}
	write(t, filepath.Join(base, "out/sim.raw"), "abc")
	f3 := r.Fingerprint()
	if f3 == f2 {
		t.Error("out file did not change fingerprint")
	}
	write(t, filepath.Join(base, "out/sim.raw"), "abcdef")
	if r.Fingerprint() == f3 {
		t.Error("out file size did not change fingerprint")
	}
}
