package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestNormalizeClawhubTarget(t *testing.T) {
	if slug, ok := normalizeClawhubTarget("clawhub:My-Skill"); !ok || slug != "my-skill" {
		t.Fatalf("unexpected normalized clawhub target: %q %v", slug, ok)
	}
	if slug, ok := normalizeClawhubTarget("weather"); !ok || slug != "weather" {
		t.Fatalf("unexpected normalized slug target: %q %v", slug, ok)
	}
	if _, ok := normalizeClawhubTarget("https://clawhub.ai/skills/weather.zip"); ok {
		t.Fatal("url should not normalize as clawhub slug")
	}
}

func TestInstallSkillWithClawhubSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))

	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+origPath)

	clawhubScript := `#!/bin/sh
slug="$2"
workdir=""
dir="skills"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--workdir" ]; then
    workdir="$2"
    shift 2
    continue
  fi
  if [ "$1" = "--dir" ]; then
    dir="$2"
    shift 2
    continue
  fi
  shift
done
[ -z "$workdir" ] && exit 2
target="$workdir/$dir/$slug"
mkdir -p "$target"
mkdir -p "$workdir/.clawhub"
cat > "$target/SKILL.md" <<'EOF'
---
name: demo-skill
description: demo
---
EOF
cat > "$workdir/.clawhub/lock.json" <<'EOF'
{"skills":[{"slug":"demo-skill","version":"1.2.3"}]}
EOF
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "clawhub"), []byte(clawhubScript), 0o755); err != nil {
		t.Fatalf("write clawhub script: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	cfg.Skills.ExternalInstalls = true
	cfg.Skills.LinkPolicy.Mode = "open"

	res, err := InstallSkill(cfg, "clawhub:demo-skill", true)
	if err != nil {
		t.Fatalf("install via clawhub source failed: %v", err)
	}
	if res == nil || strings.TrimSpace(res.Name) != "demo-skill" {
		t.Fatalf("unexpected install result: %#v", res)
	}
	if !strings.HasPrefix(res.Report.ResolvedTarget, "clawhub:") {
		t.Fatalf("expected clawhub source marker, got %q", res.Report.ResolvedTarget)
	}
	dirs, err := EnsureStateDirs()
	if err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	meta, err := readMetadata(filepath.Join(res.InstallPath, metadataFileName))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if meta.ClawhubSlug != "demo-skill" || meta.ClawhubVersion != "1.2.3" {
		t.Fatalf("unexpected clawhub metadata: %#v", meta)
	}
	if _, err := os.Stat(filepath.Join(dirs.ToolsDir, "clawhub", "managed-lock.json")); err != nil {
		t.Fatalf("expected managed clawhub lock index: %v", err)
	}
}

func TestExtractPinnedSourceHash(t *testing.T) {
	hash := strings.Repeat("a", 64)
	cases := []struct {
		target string
		ok     bool
	}{
		{target: "https://example.com/skill.zip#sha256=" + hash, ok: true},
		{target: "https://example.com/skill.zip#sha256=abcd", ok: false},
		{target: "https://example.com/skill.zip#sha512=" + hash, ok: false},
		{target: "clawhub:demo-skill", ok: false},
	}
	for _, tc := range cases {
		_, ok := extractPinnedSourceHash(tc.target)
		if ok != tc.ok {
			t.Fatalf("target=%q expected ok=%v got %v", tc.target, tc.ok, ok)
		}
	}
}
