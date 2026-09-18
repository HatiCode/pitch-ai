// Package web embeds the built single-page application into the server binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the built single-page application bundle.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// dist is embedded at compile time; a failure here is a build defect.
		panic(err)
	}
	return sub
}
