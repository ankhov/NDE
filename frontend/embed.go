//go:build gui

// Package frontend embeds the built Vite assets (frontend/dist) so the Wails
// binary is self-contained. The "gui" build tag excludes it from the headless
// build, which has no GUI assets to serve.
//
// Build the frontend first:  (cd frontend && npm run build)
// Then build the GUI:        go build -tags=gui ./cmd/app
package frontend

import "embed"

// Dist is the embedded production frontend (frontend/dist).
//go:embed all:dist
var Dist embed.FS
