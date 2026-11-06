// Package web holds the browser frontend, embedded into the server binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Static is the frontend's file tree (index.html at the root).
var Static, _ = fs.Sub(files, "static")
