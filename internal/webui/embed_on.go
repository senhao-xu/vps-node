//go:build embed_ui

package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

const embedded = true

func uiFS() (fs.FS, bool) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}
