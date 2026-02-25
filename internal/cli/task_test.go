package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestTaskStatusCLI(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"node":{"clawId":"c1","instanceId":"i1"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	tl, err := timeline.NewTimelineService(filepath.Join(cfgDir, "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	if err := tl.CreateCascadeTask(&timeline.CascadeTaskRecord{
		TaskID:   "t1",
		TraceID:  "trace-xyz",
		Sequence: 1,
		Title:    "Collect inputs",
	}); err != nil {
		t.Fatalf("create cascade task: %v", err)
	}
	if _, err := tl.AdvanceCascadeTask("trace-xyz", "t1", "pending", "running", "manager", "start", `{}`, "idem-x1"); err != nil {
		t.Fatalf("advance cascade task: %v", err)
	}
	_ = tl.Close()

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t, "task", "status", "--trace=trace-xyz", "--json")
	if err != nil {
		t.Fatalf("task status json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nout=%s", err, out)
	}
	if payload["traceId"] != "trace-xyz" {
		t.Fatalf("expected traceId trace-xyz, got %+v", payload["traceId"])
	}
	if payload["taskCount"] == nil || payload["transitionCount"] == nil {
		t.Fatalf("expected task/transition counts in output, got %v", payload)
	}

	plain, err := runRootCommand(t, "task", "status", "--trace=trace-xyz", "--json=false")
	if err != nil {
		t.Fatalf("task status text: %v", err)
	}
	if !strings.Contains(plain, "Trace: trace-xyz") {
		t.Fatalf("unexpected text output: %s", plain)
	}
}

func TestTaskStatusRequiresTrace(t *testing.T) {
	if _, err := runRootCommand(t, "task", "status", "--trace="); err == nil {
		t.Fatal("expected --trace required error")
	}
}
