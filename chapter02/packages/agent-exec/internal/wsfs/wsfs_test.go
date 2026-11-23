package wsfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func openRootFD(w *Workspace) (int, error) {
	return unix.Open(w.Root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
}

func closeFD(fd int) { unix.Close(fd) }

func newRoot(t *testing.T) (*Workspace, string) {
	t.Helper()
	root := t.TempDir()
	return New(root), root
}

func put(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRel(t *testing.T) {
	w := New("/workspace")
	ok := map[string]string{
		"/workspace":                 ".",
		"/workspace/src/a.py":        "src/a.py",
		"/workspace/src/../.input/x": ".input/x",
		"/workspace//src/":           "src",
	}
	for in, want := range ok {
		got, err := w.Rel(in)
		if err != nil || got != want {
			t.Errorf("Rel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"/etc/passwd", "/workspace/../etc/passwd", "/workspacex/a", "src/a.py", ""} {
		if _, err := w.Rel(in); !errors.Is(err, ErrOutside) {
			t.Errorf("Rel(%q) must be outside, got %v", in, err)
		}
	}
}

func TestSymlinkEscape(t *testing.T) {
	w, root := newRoot(t)
	outside := t.TempDir()
	put(t, filepath.Join(outside, "secret"), "s3cret")
	put(t, filepath.Join(root, "in/real.txt"), "real")
	if err := os.Symlink(outside, filepath.Join(root, "dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "in/filelink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../..", filepath.Join(root, "in/up")); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Stat("in/real.txt"); err != nil {
		t.Fatalf("regular file: %v", err)
	}
	for _, rel := range []string{"dirlink", "dirlink/secret", "in/filelink", "in/up", "in/up/etc/passwd", "../x"} {
		if _, err := w.Stat(rel); err == nil {
			t.Errorf("Stat(%q) followed a symlink out of the workspace", rel)
		}
		if f, err := w.OpenRegular(rel); err == nil {
			f.Close()
			t.Errorf("OpenRegular(%q) followed a symlink out of the workspace", rel)
		}
	}
	// Writing through a symlinked directory must not create anything outside.
	if err := w.WriteExcl("dirlink/planted", []byte("x")); err == nil {
		t.Error("WriteExcl through a symlink succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "planted")); err == nil {
		t.Error("a file was planted outside the workspace")
	}
	if err := w.Mkdir("dirlink/sub", 0o755); err == nil {
		t.Error("Mkdir through a symlink succeeded")
	}
	if _, err := w.Lock("dirlink", true, 10*time.Millisecond); err == nil {
		t.Error("Lock through a symlink succeeded")
	}
}

func TestFallbackWalkRefusesSymlinks(t *testing.T) {
	w, root := newRoot(t)
	outside := t.TempDir()
	put(t, filepath.Join(outside, "secret"), "x")
	put(t, filepath.Join(root, "a/b.txt"), "b")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	fd, err := openRootFD(w)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFD(fd)
	if _, err := walk(fd, "link/secret", 0, 0); err == nil {
		t.Error("walk followed a directory symlink")
	}
	good, err := walk(fd, "a/b.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	closeFD(good)
}

func TestExecDirAndWrites(t *testing.T) {
	w, root := newRoot(t)
	if err := os.Mkdir(filepath.Join(root, ".output"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.CreateExecDir("e1", true, false); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"in", "out"} {
		if st, err := os.Stat(filepath.Join(root, ".output/exec/e1", d)); err != nil || !st.IsDir() {
			t.Errorf("%s missing", d)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".output/exec/e1/dl")); err == nil {
		t.Error("dl/ only for install")
	}
	if err := w.CreateExecDir("e1", true, false); !errors.Is(err, os.ErrExist) {
		t.Errorf("second create: %v", err)
	}
	if err := w.CreateExecDir("e2", false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".output/exec/e2/in")); err == nil {
		t.Error("no in/ for install")
	}
	if err := w.WriteExcl(".output/exec/e1/meta.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteExcl(".output/exec/e1/meta.json", []byte("{}")); err == nil {
		t.Error("meta.json must be written once")
	}
}

func TestCreateExecDirNeedsOutputDir(t *testing.T) {
	w, _ := newRoot(t)
	if err := w.CreateExecDir("e1", true, false); err == nil {
		t.Fatal("must not create .output itself")
	}
}

func TestCopyFileAndCap(t *testing.T) {
	w, root := newRoot(t)
	put(t, filepath.Join(root, "src.txt"), "hello")
	if err := os.Mkdir(filepath.Join(root, "dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := w.CopyFile("src.txt", "dst/a", 100)
	if err != nil || res.Size != 5 || res.SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("copy %+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "dst/a")); string(b) != "hello" {
		t.Fatal("content")
	}
	if _, err := w.CopyFile("src.txt", "dst/a", 100); err == nil {
		t.Error("copy must not overwrite")
	}
	if _, err := w.CopyFile("src.txt", "dst/b", 4); !errors.Is(err, ErrTooBig) {
		t.Errorf("cap: %v", err)
	}
	if _, err := w.CopyFile("dst", "dst/c", 100); err == nil {
		t.Error("a directory is not copied")
	}
	h, err := w.HashFile("src.txt")
	if err != nil || h.SHA256 != res.SHA256 {
		t.Errorf("hash %+v %v", h, err)
	}
}

func TestLocks(t *testing.T) {
	w, root := newRoot(t)
	if err := os.Mkdir(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	sh1, err := w.Lock("build", false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sh2, err := w.Lock("build", false, time.Second)
	if err != nil {
		t.Fatalf("shared locks must coexist: %v", err)
	}
	if _, err := w.Lock("build", true, 50*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("exclusive against shared: %v", err)
	}
	sh1.Release()
	sh2.Release()
	ex, err := w.Lock("build", true, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Lock("build", false, 50*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("shared against exclusive: %v", err)
	}
	ex.Release()
	ex.Release() // idempotent
	if l, err := w.Lock("build", true, time.Second); err != nil {
		t.Fatalf("after release: %v", err)
	} else {
		l.Release()
	}
	if _, err := w.Lock("missing", true, time.Second); err == nil || strings.Contains(err.Error(), "lock is held") {
		t.Fatalf("missing folder: %v", err)
	}
}

func TestDirSize(t *testing.T) {
	w, root := newRoot(t)
	put(t, filepath.Join(root, "out/a"), "12345")
	put(t, filepath.Join(root, "out/sub/b"), "123")
	if got := w.DirSize("out"); got != 8 {
		t.Errorf("size %d", got)
	}
	if got := w.DirSize("none"); got != 0 {
		t.Errorf("missing dir %d", got)
	}
}

func TestReadDirAndReadFile(t *testing.T) {
	w, root := newRoot(t)
	put(t, filepath.Join(root, "d/b"), "x")
	put(t, filepath.Join(root, "d/a"), "yy")
	names, err := w.ReadDir("d")
	if err != nil || strings.Join(names, ",") != "a,b" {
		t.Fatalf("%v %v", names, err)
	}
	if b, err := w.ReadFile("d/a", 10); err != nil || string(b) != "yy" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := w.ReadFile("d/a", 1); !errors.Is(err, ErrTooBig) {
		t.Fatalf("cap %v", err)
	}
	if _, err := w.ReadFile("d", 10); err == nil {
		t.Fatal("a directory is not a file")
	}
}
