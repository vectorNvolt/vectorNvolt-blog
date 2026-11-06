//go:build windows

package config

import (
	"errors"
	"io/fs"
)

// StatFile has no owner on Windows; the Linux tools never run there. It exists so that this package compiles for
// refresh_endpoints.exe, which only uses ParseEndpoints.
func StatFile(path string) (uid int, mode fs.FileMode, err error) {
	return 0, 0, errors.New("file owners are not available on Windows")
}
