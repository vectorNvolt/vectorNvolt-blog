// Package ui embeds the built browser bundle (static/index.html, app.js, app.css).
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var content embed.FS

// FS returns the bundle rooted at static/.
func FS() fs.FS {
	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
