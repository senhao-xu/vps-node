//go:build !embed_ui

package webui

import "io/fs"

const embedded = false

func uiFS() (fs.FS, bool) {
	return nil, false
}
