package engine

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Demux splits the multiplexed stream of a non-tty container (8-byte header: stream type,
// three zero bytes, big-endian length) into stdout and stderr. SFR-AGE-012.
// A clean end of stream (EOF at a frame boundary) returns nil.
func Demux(r io.Reader, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(r, hdr); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var w io.Writer
		switch hdr[0] {
		case 0, 1:
			w = stdout
		case 2:
			w = stderr
		default:
			return fmt.Errorf("unknown stream type %d", hdr[0])
		}
		size := int64(binary.BigEndian.Uint32(hdr[4:8]))
		if _, err := io.CopyN(w, r, size); err != nil {
			return err
		}
	}
}
