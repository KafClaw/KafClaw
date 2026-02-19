package webassets

import "embed"

// Files contains the embedded dashboard and report template assets.
//
// Keep this broad enough so web page updates are automatically packaged in binaries.
//
//go:embed *.html templates
var Files embed.FS
