package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeDashboardAssetServesEmbeddedHTML(t *testing.T) {
	rr := httptest.NewRecorder()
	serveDashboardAsset(rr, "index.html")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("expected html content type, got %q", got)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("expected non-empty response body")
	}
}

func TestServeDashboardAssetMissingFile(t *testing.T) {
	rr := httptest.NewRecorder()
	serveDashboardAsset(rr, "missing-page.html")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}
