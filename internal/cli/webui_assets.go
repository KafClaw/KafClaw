package cli

import (
	"mime"
	"net/http"
	"path/filepath"

	webassets "github.com/KafClaw/KafClaw/web"
)

// vendorAssetHandler serves the embedded frontend vendor assets (Tailwind, Vue,
// d3, dagre-d3, JetBrains Mono fonts) under /vendor/. The embed FS is rooted at
// web/, so the requested path /vendor/<x> resolves to the embedded vendor/<x>.
// Serving these locally keeps the dashboard offline-capable with no CDN calls.
func vendorAssetHandler() http.Handler {
	return http.FileServer(http.FS(webassets.Files))
}

func serveDashboardAsset(w http.ResponseWriter, name string) {
	body, err := webassets.Files.ReadFile(name)
	if err != nil {
		http.Error(w, "dashboard asset missing", http.StatusInternalServerError)
		return
	}
	if ctype := mime.TypeByExtension(filepath.Ext(name)); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	_, _ = w.Write(body)
}
