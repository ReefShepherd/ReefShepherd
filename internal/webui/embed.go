// Package webui serves the built web UI that is embedded in the binary.
package webui

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded UI, warning through logger about a shell that has no content security policy.
func Handler(logger *slog.Logger) http.Handler {
	// The dist directory is guaranteed by the embed directive, so Sub cannot fail.
	sub, _ := fs.Sub(distFS, "dist")
	return NewHandler(sub, logger)
}
