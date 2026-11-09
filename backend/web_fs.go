package backend

import (
	"embed"
	"io/fs"
	"os"
)

// In case embedded assets are bundled during release build
//
//go:embed all:web_dist
var embeddedDist embed.FS

// GetWebFileSystem checks disk first (live dev reload), falling back to embed
func GetWebFileSystem() fs.FS {
	// 1. Check if running from backend/ (dev mode: ../web)
	if _, err := os.Stat("../web/index.html"); err == nil {
		return os.DirFS("../web")
	}

	// 2. Check if running from root cloud/ (dev mode: ./web)
	if _, err := os.Stat("web/index.html"); err == nil {
		return os.DirFS("web")
	}

	// 3. Fall back to embedded assets for compiled single-binary release
	sub, err := fs.Sub(embeddedDist, "web_dist")
	if err != nil {
		return os.DirFS(".")
	}
	return sub
}
