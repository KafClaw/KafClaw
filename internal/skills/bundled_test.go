package skills

import "testing"

func TestBundledArtifactsPresent(t *testing.T) {
	if err := ValidateBundledArtifacts(""); err != nil {
		t.Fatalf("bundled artifacts validation failed: %v", err)
	}
}
