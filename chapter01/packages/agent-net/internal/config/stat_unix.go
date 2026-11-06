//go:build !windows

package config

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// StatFile reports the owner and mode of a file (following symbolic links).
func StatFile(path string) (uid int, mode fs.FileMode, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("cannot read the owner of %s", path)
	}
	return int(st.Uid), fi.Mode(), nil
}
