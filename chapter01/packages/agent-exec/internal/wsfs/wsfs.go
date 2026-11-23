// Package wsfs is the only way run_ephemeral_container touches /workspace.
// The agent controls the workspace and can plant symlinks, so every path is resolved
// below the workspace root without following symlinks (openat2 RESOLVE_BENEATH |
// RESOLVE_NO_SYMLINKS, with a component-wise O_NOFOLLOW walk where openat2 is missing).
// SSR-AGE-013 (no symlink escape), SSR-AGE-010 (exec dir record), SSR-AGE-011 (locks).
package wsfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var (
	ErrOutside = errors.New("path is outside the workspace")
	ErrBusy    = errors.New("lock is held")
	ErrTooBig  = errors.New("size cap exceeded")
	ErrNotReg  = errors.New("not a regular file")
)

// Workspace is the workspace root as seen by the tool (default /workspace).
type Workspace struct{ Root string }

func New(root string) *Workspace { return &Workspace{Root: filepath.Clean(root)} }

// Rel turns an absolute path under the root into a clean root-relative path ("." for the root).
// SSR-AGE-013: lexical check here, the symlink check happens in every open below.
func (w *Workspace) Rel(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: %q is not absolute", ErrOutside, p)
	}
	c := filepath.Clean(p)
	if c == w.Root {
		return ".", nil
	}
	prefix := w.Root
	if prefix != "/" {
		prefix += "/"
	}
	if !strings.HasPrefix(c, prefix) {
		return "", fmt.Errorf("%w: %q", ErrOutside, p)
	}
	return strings.TrimPrefix(c, prefix), nil
}

// Abs is the absolute path of a root-relative path (for display, bind sources, size walks).
func (w *Workspace) Abs(rel string) string { return filepath.Join(w.Root, rel) }

func cleanRel(rel string) (string, error) {
	c := filepath.Clean(rel)
	if filepath.IsAbs(c) || c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("%w: %q", ErrOutside, rel)
	}
	return c, nil
}

// openat opens rel below the root; the result never passed through a symlink.
func (w *Workspace) openat(rel string, flags int, mode uint32) (int, error) {
	rel, err := cleanRel(rel)
	if err != nil {
		return -1, err
	}
	root, err := unix.Open(w.Root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	defer unix.Close(root)
	how := &unix.OpenHow{
		Flags:   uint64(flags | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	}
	if flags&unix.O_CREAT != 0 {
		how.Mode = uint64(mode)
	}
	for {
		fd, err := unix.Openat2(root, rel, how)
		if err == nil {
			return fd, nil
		}
		if err == unix.EINTR {
			continue
		}
		if err == unix.ENOSYS {
			return walk(root, rel, flags, mode)
		}
		return -1, err
	}
}

// walk is the openat2 fallback: one openat per component with O_NOFOLLOW.
func walk(root int, rel string, flags int, mode uint32) (int, error) {
	if rel == "." {
		return unix.Openat(root, ".", flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, mode)
	}
	parts := strings.Split(rel, "/")
	cur := root
	for i, p := range parts {
		f := unix.O_NOFOLLOW | unix.O_CLOEXEC
		m := uint32(0)
		if i == len(parts)-1 {
			f |= flags
			m = mode
		} else {
			f |= unix.O_RDONLY | unix.O_DIRECTORY
		}
		fd, err := unix.Openat(cur, p, f, m)
		if cur != root {
			unix.Close(cur)
		}
		if err != nil {
			return -1, err
		}
		cur = fd
	}
	return cur, nil
}

// Info is the part of stat the tool needs.
type Info struct {
	Mode uint32
	Size int64
}

func (i Info) IsDir() bool     { return i.Mode&unix.S_IFMT == unix.S_IFDIR }
func (i Info) IsRegular() bool { return i.Mode&unix.S_IFMT == unix.S_IFREG }

// Stat never follows symlinks; a symlink at rel is ELOOP.
func (w *Workspace) Stat(rel string) (Info, error) {
	fd, err := w.openat(rel, unix.O_PATH, 0)
	if err != nil {
		return Info{}, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return Info{}, err
	}
	if st.Mode&unix.S_IFMT == unix.S_IFLNK {
		return Info{}, unix.ELOOP
	}
	return Info{Mode: uint32(st.Mode), Size: st.Size}, nil
}

// Exists is conservative: anything but "not there" (including a symlink in the way) counts as existing.
func (w *Workspace) Exists(rel string) bool {
	_, err := w.Stat(rel)
	return !(err == unix.ENOENT || err == unix.ENOTDIR)
}

func (w *Workspace) OpenFile(rel string, flags int, mode uint32) (*os.File, error) {
	fd, err := w.openat(rel, flags, mode)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: rel, Err: err}
	}
	return os.NewFile(uintptr(fd), w.Abs(rel)), nil
}

// OpenRegular opens an existing regular file for reading.
func (w *Workspace) OpenRegular(rel string) (*os.File, error) {
	f, err := w.OpenFile(rel, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, &fs.PathError{Op: "open", Path: rel, Err: ErrNotReg}
	}
	return f, nil
}

func (w *Workspace) ReadDir(rel string) ([]string, error) {
	f, err := w.OpenFile(rel, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (w *Workspace) ReadFile(rel string, max int64) ([]byte, error) {
	f, err := w.OpenRegular(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, ErrTooBig
	}
	return data, nil
}

// Mkdir creates one directory; the parent is resolved without symlinks. EEXIST is returned as is.
func (w *Workspace) Mkdir(rel string, perm uint32) error {
	rel, err := cleanRel(rel)
	if err != nil {
		return err
	}
	if rel == "." {
		return unix.EEXIST
	}
	pfd, err := w.openat(filepath.Dir(rel), unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(pfd)
	return unix.Mkdirat(pfd, filepath.Base(rel), perm)
}

// WriteExcl writes a new file that must not exist (meta.json and result.json are written once).
func (w *Workspace) WriteExcl(rel string, data []byte) error {
	f, err := w.OpenFile(rel, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ExecRel is the root-relative exec dir of an execution.
func ExecRel(execID string) string { return ".output/exec/" + execID }

// CreateExecDir creates .output/exec/<id>/ with in/ (not install), out/ and dl/ (install only).
// The folder must not exist (os.ErrExist). SFR-AGE-003/SSR-AGE-010: unique, never reused.
func (w *Workspace) CreateExecDir(execID string, withIn, withDl bool) error {
	if err := w.Mkdir(".output/exec", 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	dir := ExecRel(execID)
	if err := w.Mkdir(dir, 0o755); err != nil {
		return err
	}
	subs := []string{"out"}
	if withIn {
		subs = append(subs, "in")
	}
	if withDl {
		subs = append(subs, "dl")
	}
	for _, s := range subs {
		if err := w.Mkdir(dir+"/"+s, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Lock is a flock on a folder (SSR-AGE-011).
type Lock struct{ fd int }

// Lock takes an exclusive or shared flock on the folder rel, waiting at most wait. ErrBusy when not available.
func (w *Workspace) Lock(rel string, exclusive bool, wait time.Duration) (*Lock, error) {
	fd, err := w.openat(rel, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	how := unix.LOCK_SH
	if exclusive {
		how = unix.LOCK_EX
	}
	deadline := time.Now().Add(wait)
	for {
		err := unix.Flock(fd, how|unix.LOCK_NB)
		if err == nil {
			return &Lock{fd: fd}, nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EINTR {
			unix.Close(fd)
			return nil, err
		}
		if time.Now().After(deadline) {
			unix.Close(fd)
			return nil, ErrBusy
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (l *Lock) Release() {
	if l != nil && l.fd >= 0 {
		unix.Close(l.fd)
		l.fd = -1
	}
}

// StageResult describes one copied or hashed input file.
type StageResult struct {
	SHA256 string
	Size   int64
}

// CopyFile copies the regular file srcRel to the new file dstRel (O_EXCL) and hashes it.
// At most remaining bytes are copied, else ErrTooBig (exc-5). SFR-AGE-007.
func (w *Workspace) CopyFile(srcRel, dstRel string, remaining int64) (StageResult, error) {
	src, err := w.OpenRegular(srcRel)
	if err != nil {
		return StageResult{}, err
	}
	defer src.Close()
	dst, err := w.OpenFile(dstRel, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0o644)
	if err != nil {
		return StageResult{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, remaining+1))
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return StageResult{}, err
	}
	if n > remaining {
		return StageResult{}, ErrTooBig
	}
	return StageResult{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}

// HashFile hashes a regular file used in place.
func (w *Workspace) HashFile(rel string) (StageResult, error) {
	f, err := w.OpenRegular(rel)
	if err != nil {
		return StageResult{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return StageResult{}, err
	}
	return StageResult{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}

// DirSize sums the sizes of the regular files below rel (symlinks are not followed, vanished
// files are skipped). Used only to watch out_max, so a path race can only skew the number.
func (w *Workspace) DirSize(rel string) int64 {
	var total int64
	_ = filepath.WalkDir(w.Abs(rel), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
