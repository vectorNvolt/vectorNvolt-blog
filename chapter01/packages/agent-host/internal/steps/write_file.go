package steps

// WriteFileCmd is the argv that writes stdin to dst as root:root with the given mode (e.g. "644").
// dst and mode are positional args ($1, $2), never interpolated into the script. The file is written
// to a temp sibling and renamed, so dst is replaced atomically. (uutils `install` cannot read /dev/stdin.)
func WriteFileCmd(dst, mode string) []string {
	return []string{"/bin/sh", "-c", `t="$1.tmp" && cat > "$t" && chown root:root "$t" && chmod "$2" "$t" && mv -f "$t" "$1"`, "sh", dst, mode}
}
