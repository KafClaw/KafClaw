package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/knowledge"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestKnowledgePublisherUsesConfiguredAndStoredProxy(t *testing.T) {
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "timeline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || !json.Valid(body) {
			t.Errorf("invalid produce request: %s %s", r.Method, body)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"topic":"knowledge.fixture","partition":0,"offset":1}`)
	}))
	defer server.Close()
	cfg := config.DefaultConfig()
	cfg.Group.LFSProxyURL = server.URL
	env := knowledge.Envelope{SchemaVersion: knowledge.CurrentSchemaVersion, Type: knowledge.TypeProposal, TraceID: "trace-fixture", Timestamp: time.Now(), IdempotencyKey: "proposal-fixture", ClawID: "claw", InstanceID: "instance"}
	if err := publishKnowledgeEnvelope(cfg, tl, "knowledge.fixture", env); err != nil {
		t.Fatal(err)
	}
	if err := tl.SetSetting("kafscale_lfs_proxy_url", server.URL); err != nil {
		t.Fatal(err)
	}
	cfg.Group.LFSProxyURL = "http://localhost:8080"
	if err := publishKnowledgeEnvelope(cfg, tl, "knowledge.fixture", env); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("produce count=%d", requests)
	}
	if err := publishKnowledgeEnvelope(cfg, tl, "", env); err == nil {
		t.Fatal("empty topic accepted")
	}
	if err := publishKnowledgeEnvelope(cfg, tl, "knowledge.fixture", knowledge.Envelope{}); err == nil {
		t.Fatal("invalid envelope accepted")
	}
	cfg.Group.LFSProxyURL = ""
	tl.SetSetting("kafscale_lfs_proxy_url", "")
	if err := publishKnowledgeEnvelope(cfg, tl, "knowledge.fixture", env); err == nil {
		t.Fatal("missing proxy accepted")
	}
	cfg.Group.LFSProxyURL = server.URL
	env.Payload = make(chan int)
	if err := publishKnowledgeEnvelope(cfg, tl, "knowledge.fixture", env); err == nil {
		t.Fatal("unserializable payload accepted")
	}
}
