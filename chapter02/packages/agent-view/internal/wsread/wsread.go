// Package wsread is the only way agent_view touches the workspace. It is read-only and exposes
// nothing but <ws>/.output/exec/<id>/. The agent controls the workspace and can plant or swap
// symlinks at any time, so every access is an fd-relative openat(O_NOFOLLOW) walk from the
// workspace root (see openBeneath): symlinks below the root are refused, nothing is re-resolved.
package wsread

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// MaxFileSize is the largest file served (64 MiB).
const MaxFileSize int64 = 64 << 20

var (
	// ErrBadRequest: malformed exec id or relative path (HTTP 400).
	ErrBadRequest = errors.New("bad request")
	// ErrNotFound: anything else that does not resolve to a servable file (HTTP 404, no reason leaked).
	ErrNotFound = errors.New("not found")
	// ErrTooBig: the file exceeds MaxFileSize (HTTP 413).
	ErrTooBig = errors.New("file too large")
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidID reports whether id is an acceptable exec id.
func ValidID(id string) bool { return idRe.MatchString(id) }

// CleanRel validates and cleans a path relative to an exec dir.
func CleanRel(rel string) (string, error) {
	if rel == "" || strings.ContainsRune(rel, 0) || strings.ContainsRune(rel, '\\') || strings.HasPrefix(rel, "/") {
		return "", ErrBadRequest
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", ErrBadRequest
		}
	}
	c := path.Clean(rel)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") || path.IsAbs(c) {
		return "", ErrBadRequest
	}
	return c, nil
}

// Reader reads exec dirs below one workspace root.
type Reader struct {
	root string // symlink-resolved workspace root
}

// New resolves ws (must be an existing directory).
func New(ws string) (*Reader, error) {
	real, err := filepath.EvalSymlinks(ws)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(real)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", ws)
	}
	return &Reader{root: abs}, nil
}

// openBeneath opens the path comps below the workspace root without ever re-resolving a path.
//
// Why: the exec dirs are writable by the agent, which runs as the same uid as this viewer, so it
// can swap any path component for a symlink between a check and the following open (TOCTOU). A
// resolve-then-open scheme would let it get any file the viewer can read served. Instead the root
// (operator-supplied, trusted, resolved once in New) is opened and every component is opened with
// openat(O_NOFOLLOW) relative to the previous directory fd, so a symlink anywhere below the root
// fails (ELOOP/ENOTDIR) and is refused. Hard links cannot escape either: the agent's /workspace is
// a separate bind mount, so link() from it to a file elsewhere fails with EXDEV.
//
// With wantDir every component, the last included, is opened as a directory. Otherwise the last is
// opened as a file (O_NONBLOCK so a planted FIFO cannot hang the server) and the caller must fstat
// it. Components must be non-empty, without "/", "." or "..". Any failure is ErrNotFound.
func openBeneath(rootPath string, comps []string, wantDir bool) (*os.File, error) {
	for _, c := range comps {
		if c == "" || c == "." || c == ".." || strings.ContainsAny(c, "/\x00") {
			return nil, ErrNotFound
		}
	}
	fd, err := syscall.Open(rootPath, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrNotFound
	}
	for i, c := range comps {
		flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
		if wantDir || i < len(comps)-1 {
			flags |= syscall.O_DIRECTORY
		} else {
			flags |= syscall.O_NONBLOCK
		}
		next, err := syscall.Openat(fd, c, flags, 0)
		syscall.Close(fd)
		if err != nil {
			return nil, ErrNotFound
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), comps[len(comps)-1]), nil
}

// openRegular opens a regular file below the root (see openBeneath) and enforces maxSize from fstat.
func (r *Reader) openRegular(comps []string, maxSize int64) (*os.File, fs.FileInfo, error) {
	f, err := openBeneath(r.root, comps, false)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, nil, ErrNotFound
	}
	if st.Size() > maxSize {
		f.Close()
		return nil, nil, ErrTooBig
	}
	return f, st, nil
}

// OpenFile opens a regular file inside the exec dir of id, read-only. Symlinks are refused.
func (r *Reader) OpenFile(id, rel string) (*os.File, fs.FileInfo, error) {
	if !ValidID(id) {
		return nil, nil, ErrBadRequest
	}
	clean, err := CleanRel(rel)
	if err != nil {
		return nil, nil, err
	}
	comps := append([]string{".output", "exec", id}, strings.Split(clean, "/")...)
	return r.openRegular(comps, MaxFileSize)
}

// Exec is one entry of the exec listing.
type Exec struct {
	ID      string          `json:"id"`
	MTime   string          `json:"mtime"`
	Result  json.RawMessage `json:"result"`
	Meta    json.RawMessage `json:"meta"`
	Inputs  []string        `json:"inputs"`
	Outputs []string        `json:"outputs"`
}

type idInfo struct {
	id    string
	mtime time.Time
}

// fstatat classifies name in the directory f without following a symlink.
func fstatat(f *os.File, name string) (syscall.Stat_t, bool) {
	// stdlib syscall has no Fstatat on linux/amd64: call newfstatat directly.
	const atSymlinkNofollow = 0x100
	var st syscall.Stat_t
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return st, false
	}
	_, _, e := syscall.Syscall6(syscall.SYS_NEWFSTATAT, f.Fd(), uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&st)), atSymlinkNofollow, 0, 0)
	runtime.KeepAlive(f)
	return st, e == 0
}

// entries lists the names in the directory comps (opened via openBeneath) whose type, without
// following symlinks, is typ (syscall.S_IFDIR or S_IFREG), with their stat.
func (r *Reader) entries(comps []string, typ uint32) (map[string]syscall.Stat_t, []string) {
	dir, err := openBeneath(r.root, comps, true)
	if err != nil {
		return nil, nil
	}
	defer dir.Close()
	des, err := dir.ReadDir(-1)
	if err != nil {
		return nil, nil
	}
	stats := map[string]syscall.Stat_t{}
	names := []string{}
	for _, e := range des {
		st, ok := fstatat(dir, e.Name())
		if !ok || st.Mode&syscall.S_IFMT != typ {
			continue
		}
		stats[e.Name()] = st
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return stats, names
}

func (r *Reader) listIDs() []idInfo {
	stats, names := r.entries([]string{".output", "exec"}, syscall.S_IFDIR)
	var out []idInfo
	for _, n := range names {
		if !ValidID(n) {
			continue
		}
		out = append(out, idInfo{n, time.Unix(int64(stats[n].Mtim.Sec), int64(stats[n].Mtim.Nsec))})
	}
	return out
}

// regularNames lists the regular files directly in the dir (symlinks and subdirs are skipped).
func (r *Reader) regularNames(comps ...string) []string {
	_, names := r.entries(comps, syscall.S_IFREG)
	if names == nil {
		return []string{}
	}
	return names
}

// jsonMax caps result.json / meta.json (1 MiB).
const jsonMax int64 = 1 << 20

// readObject returns the file as raw JSON when it is a valid JSON object, else nil (-> null).
func (r *Reader) readObject(id, name string) json.RawMessage {
	f, _, err := r.openRegular([]string{".output", "exec", id, name}, jsonMax)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, jsonMax))
	if err != nil || !json.Valid(data) {
		return nil
	}
	trimmed := strings.TrimLeft(string(data), " \t\r\n")
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	return json.RawMessage(data)
}

// ListExecs lists the exec dirs, newest first. A missing .output/exec gives an empty list.
func (r *Reader) ListExecs() []Exec {
	ids := r.listIDs()
	sort.Slice(ids, func(i, j int) bool {
		if !ids[i].mtime.Equal(ids[j].mtime) {
			return ids[i].mtime.After(ids[j].mtime)
		}
		return ids[i].id > ids[j].id
	})
	list := make([]Exec, 0, len(ids))
	for _, it := range ids {
		list = append(list, Exec{
			ID:      it.id,
			MTime:   it.mtime.UTC().Format(time.RFC3339),
			Result:  r.readObject(it.id, "result.json"),
			Meta:    r.readObject(it.id, "meta.json"),
			Inputs:  r.regularNames(".output", "exec", it.id, "in"),
			Outputs: r.regularNames(".output", "exec", it.id, "out"),
		})
	}
	return list
}

// Fingerprint changes whenever an exec dir, its result.json or its out/ files change
// (names, mtimes, sizes).
func (r *Reader) Fingerprint() string {
	ids := r.listIDs()
	sort.Slice(ids, func(i, j int) bool { return ids[i].id < ids[j].id })
	h := fnv.New64a()
	add := func(format string, a ...any) { fmt.Fprintf(h, format, a...) }
	for _, it := range ids {
		add("d %s %d\n", it.id, it.mtime.UnixNano())
		files, _ := r.entries([]string{".output", "exec", it.id}, syscall.S_IFREG)
		if st, ok := files["result.json"]; ok {
			add("r %d %d\n", int64(st.Mtim.Sec)*1e9+int64(st.Mtim.Nsec), st.Size)
		}
		outs, onames := r.entries([]string{".output", "exec", it.id, "out"}, syscall.S_IFREG)
		for _, n := range onames {
			add("o %s %d %d\n", n, int64(outs[n].Mtim.Sec)*1e9+int64(outs[n].Mtim.Nsec), outs[n].Size)
		}
	}
	return fmt.Sprintf("%d:%x", len(ids), h.Sum64())
}
