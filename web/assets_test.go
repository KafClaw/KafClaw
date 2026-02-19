package webassets

import "testing"

func TestEmbeddedWebAssetsIncludeDashboardPages(t *testing.T) {
	t.Helper()
	required := []string{
		"index.html",
		"timeline.html",
		"group.html",
		"approvals.html",
		"templates/kshark_report.html",
	}
	for _, name := range required {
		b, err := Files.ReadFile(name)
		if err != nil {
			t.Fatalf("embedded asset missing %q: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("embedded asset is empty %q", name)
		}
	}
}
