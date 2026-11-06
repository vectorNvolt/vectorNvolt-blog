// Build context assembly: the Windows-side build inputs as an in-memory tar for `docker build -`.
package services

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// BuildContextTar packs the named files of dir at the tar root. The tar is deterministic: sorted
// names, mode 0644, uid/gid 0, fixed modification time.
func BuildContextTar(dir string, files []string) ([]byte, error) {
	names := append([]string(nil), files...)
	sort.Strings(names)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reading build context file: %w", err)
		}
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}
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
