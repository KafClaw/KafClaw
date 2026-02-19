package cli

import (
	"mime"
	"net/http"
	"path/filepath"

	webassets "github.com/KafClaw/KafClaw/web"
)

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
