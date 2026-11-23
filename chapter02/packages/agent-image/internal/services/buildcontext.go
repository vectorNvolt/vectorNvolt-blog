// Build context assembly: the build inputs as an in-memory tar for `docker build -`.
package services

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-image/internal/config"
)

// BaseHash is the first config.BaseHashLen hex characters of the sha256 of the Dockerfile.base
// content with CRLF normalized to LF (so a checkout with different line endings yields the same hash).
func BaseHash(content []byte) string {
	sum := sha256.Sum256(bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n")))
	return hex.EncodeToString(sum[:])[:config.BaseHashLen]
}

// BuildContextTar packs the named files of dir into the tar. Names are slash-separated paths
// relative to dir (e.g. "agent_graph/schemas/x.yml") and become the tar entry names. The tar is
// deterministic: sorted names, file entries only, mode 0644 (0755 for config.BinFiles), uid/gid 0, fixed modification time.
func BuildContextTar(dir string, files []string) ([]byte, error) {
	names := append([]string(nil), files...)
	sort.Strings(names)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, fmt.Errorf("reading build context file: %w", err)
		}
		var mode int64 = 0644
		for _, b := range config.BinFiles {
			if name == b {
				mode = 0755
			}
		}
		hdr := &tar.Header{Name: name, Mode: mode, Size: int64(len(data)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PackageFiles lists the files of the Python package directory pkg (in dir) that belong in the
// build context, as sorted slash-separated paths starting with pkg: every *.py file at any depth
// and every *.yml file directly in schemas/. Directories named __pycache__ or *.egg-info are
// skipped; any other file is left out. A selected file must be a regular file after symbolic
// links are followed. Symbolic links to directories inside the package are not followed.
func PackageFiles(dir, pkg string) ([]string, error) {
	root, err := filepath.EvalSymlinks(filepath.Join(dir, pkg))
	if err != nil {
		return nil, fmt.Errorf("reading build context directory %s: %w", pkg, err)
	}
	var out []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p != root && (d.Name() == "__pycache__" || strings.HasSuffix(d.Name(), ".egg-info")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !packageFileSelected(rel) {
			return nil
		}
		fi, err := os.Stat(p)
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("build file %s is not a regular file", path.Join(pkg, rel))
		}
		out = append(out, path.Join(pkg, rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// packageFileSelected applies the file rule of PackageFiles to a slash path relative to the package.
func packageFileSelected(rel string) bool {
	if strings.HasSuffix(rel, ".py") {
		return true
	}
	d, f := path.Split(rel)
	return d == "schemas/" && strings.HasSuffix(f, ".yml")
}
