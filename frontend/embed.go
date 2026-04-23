package frontend

import "embed"

// Dist contains the compiled frontend assets embedded into the Go binary.
// Rebuild frontend/dist before compiling the server so the latest assets are included.
//
//go:embed all:dist
var Dist embed.FS
