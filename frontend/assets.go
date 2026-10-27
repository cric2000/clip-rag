package frontend

import "embed"

// Assets are the compiled local UI; run npm run build before building Go.
//
//go:embed all:dist
var Assets embed.FS
