package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/provider"
)

type observationProvider struct {
	call func(*provider.ChatRequest) (*provider.ChatResponse, error)
}

func (p *observationProvider) Chat(_ context.Context, r *provider.ChatRequest) (*provider.ChatResponse, error) {
	return p.call(r)
}
func (p *observationProvider) DefaultModel() string { return "observer-model" }
func (p *observationProvider) Transcribe(context.Context, *provider.AudioRequest) (*provider.AudioResponse, error) {
	return nil, errors.New("unsupported")
}
func (p *observationProvider) Speak(context.Context, *provider.TTSRequest) (*provider.TTSResponse, error) {
	return nil, errors.New("unsupported")
}

func TestObservationCompressionAndReflection(t *testing.T) {
	db := setupObserverDB(t)
	defer db.Close()
	db.SetMaxOpenConns(1)
	calls := 0
	prov := &observationProvider{call: func(r *provider.ChatRequest) (*provider.ChatResponse, error) {
		calls++
		if r.Model != "observer-model" && r.Model != "configured" {
			t.Errorf("model: %s", r.Model)
		}
		if len(r.Messages) != 2 || r.Messages[1].Content == "" {
			t.Error("missing compression input")
		}
		return &provider.ChatResponse{Content: "## 2026-10-01\n- [HIGH] User prefers Go\n- [LOW] User likes tea"}, nil
	}}
	if NewObserver(ObserverConfig{}, prov, db) != nil || NewObserver(ObserverConfig{Enabled: true}, nil, db) != nil || NewObserver(ObserverConfig{Enabled: true}, prov, nil) != nil {
		t.Fatal("disabled observer created")
	}
	defaults := NewObserver(ObserverConfig{Enabled: true}, prov, db)
	if defaults.config.MessageThreshold != 50 || defaults.config.MaxObservations != 200 {
		t.Fatal("defaults changed")
	}
	o := NewObserver(ObserverConfig{Enabled: true, MessageThreshold: 2, MaxObservations: 2}, prov, db)
	ctx := context.Background()
	if err := o.Observe(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if err := o.Reflect(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"s", "other"} {
		o.EnqueueMessage(s, "user", "A sufficiently long conversation message for memory")
	}
	if o.ShouldObserve("s") {
		t.Fatal("observed below threshold")
	}
	o.EnqueueMessage("s", "assistant", "Another sufficiently long response for observation")
	if !o.ShouldObserve("s") {
		t.Fatal("threshold not reached")
	}
	if err := o.Observe(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if o.ShouldObserve("s") || !o.ShouldReflect("s") {
		t.Fatal("queue or reflection threshold wrong")
	}
	obs, err := o.LoadObservations("s")
	if err != nil || len(obs) != 2 || obs[0].ID == "" {
		t.Fatalf("observations: %+v %v", obs, err)
	}
	all, err := o.AllObservations(0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all: %+v %v", all, err)
	}
	all, err = o.AllObservations(1)
	if err != nil || len(all) != 1 {
		t.Fatal("limit not enforced")
	}
	status := o.Status()
	if !status.Enabled || status.ObservationCount != 2 || status.QueueDepth != 1 {
		t.Fatalf("status: %+v", status)
	}
	o.config.Model = "configured"
	prov.call = func(r *provider.ChatRequest) (*provider.ChatResponse, error) {
		calls++
		if r.Model != "configured" || r.MaxTokens != 3000 {
			t.Error("reflection model/budget")
		}
		return &provider.ChatResponse{Content: "- [HIGH] Consolidated preference"}, nil
	}
	if err := o.Reflect(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if o.ObservationCount("s") != 1 || calls != 2 {
		t.Fatal("reflection did not replace observations")
	}
	prov.call = func(*provider.ChatRequest) (*provider.ChatResponse, error) { return nil, errors.New("offline") }
	o.config.MaxObservations = 1
	if err := o.Reflect(ctx, "s"); err == nil {
		t.Fatal("reflection error hidden")
	}
	o.EnqueueMessage("other", "user", "Another long message that triggers compression")
	if err := o.Observe(ctx, "other"); err == nil {
		t.Fatal("compression error hidden")
	}
	db.Close()
	if err := o.Observe(ctx, "s"); err == nil {
		t.Fatal("closed database accepted")
	}
	if err := o.Reflect(ctx, "s"); err == nil {
		t.Fatal("closed database accepted")
	}
	if _, err := o.AllObservations(1); err == nil {
		t.Fatal("closed database accepted")
	}
	if o.ShouldObserve("s") {
		t.Fatal("closed db returned true")
	}
	o.EnqueueMessage("s", "user", strings.Repeat("x", 30))
	var nilObserver *Observer
	if err := nilObserver.Reflect(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := nilObserver.AllObservations(0); err != nil {
		t.Fatal(err)
	}
	if nilObserver.Status().Enabled {
		t.Fatal("nil enabled")
	}
}

func TestQdrantHTTPContract(t *testing.T) {
	for _, mode := range []string{"existing", "create", "status", "malformed", "offline"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "status" {
					w.WriteHeader(503)
					w.Write([]byte("unavailable"))
					return
				}
				if r.Method == "GET" {
					if mode == "create" {
						w.WriteHeader(404)
					}
					return
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Error("missing JSON type")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if strings.HasSuffix(r.URL.Path, "/search") {
					if body["limit"] != float64(3) || body["with_payload"] != true {
						t.Errorf("search body: %v", body)
					}
					if mode == "malformed" {
						w.Write([]byte("invalid"))
						return
					}
					w.Write([]byte(`{"result":[{"id":"id","score":0.9,"payload":{"content":"memory"}}]}`))
					return
				}
				if strings.HasSuffix(r.URL.Path, "/points") {
					points, _ := body["points"].([]any)
					if len(points) != 1 || r.URL.Query().Get("wait") != "true" {
						t.Errorf("upsert body: %v", body)
					}
				} else if body["vectors"].(map[string]any)["size"] != float64(2) {
					t.Errorf("collection dimensions: %v", body)
				}
			}))
			defer server.Close()
			store := NewQdrantStore(server.URL, "memory", 2)
			if mode == "offline" {
				server.Close()
			}
			ctx := context.Background()
			err := store.EnsureCollection(ctx)
			if (err != nil) != (mode == "offline" || mode == "status") {
				t.Fatalf("ensure: %v", err)
			}
			err = store.Upsert(ctx, "id", []float32{1, 0}, map[string]any{"content": "memory"})
			if (err != nil) != (mode == "offline" || mode == "status") {
				t.Fatalf("upsert: %v", err)
			}
			results, err := store.Search(ctx, []float32{1, 0}, 3)
			if mode == "existing" || mode == "create" {
				if err != nil || len(results) != 1 || results[0].ID != "id" || results[0].Payload["content"] != "memory" {
					t.Fatalf("search: %+v %v", results, err)
				}
			} else if err == nil {
				t.Fatal("search failure hidden")
			}
			if err := store.Upsert(ctx, "id", nil, map[string]any{"invalid": make(chan int)}); err == nil {
				t.Fatal("invalid payload accepted")
			}
		})
	}
}

func TestKafGraphIncrementalSync(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := NewMemoryService(NewSQLiteVecStore(db, 2), nil)
	old := time.Now().Add(-time.Hour)
	nodes := []kgNode{{ID: "first", CreatedAt: old, Properties: map[string]any{"title": "Title", "category": "ops", "memory_id": "m1", "text": "graph note"}}, {ID: "fallback", CreatedAt: old, Properties: map[string]any{"text": "fallback note"}}, {ID: "empty", CreatedAt: old}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("label") != "MemoryItem" {
			t.Error("label missing")
		}
		json.NewEncoder(w).Encode(nodes)
	}))
	defer server.Close()
	if NewKafGraphClient(KafGraphConfig{}, svc) != nil || NewKafGraphClient(KafGraphConfig{URL: server.URL}, nil) != nil {
		t.Fatal("invalid client accepted")
	}
	c := NewKafGraphClient(KafGraphConfig{URL: server.URL + "/"}, svc)
	if c.config.SyncInterval != 5*time.Minute {
		t.Fatal("default interval changed")
	}
	n, err := c.SyncOnce(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("initial sync: %d %v", n, err)
	}
	n, err = c.SyncOnce(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("incremental sync: %d %v", n, err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM memory_chunks WHERE source IN ('kafgraph:m1','kafgraph:fallback')").Scan(&count); err != nil || count != 2 {
		t.Fatalf("indexed source IDs: %d %v", count, err)
	}
	c.lastSync = time.Time{}
	c.service = NewMemoryService(&fakeVectorStore{upsertErr: errors.New("write failed")}, &fakeEmbedder{})
	if n, err := c.SyncOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("failed stores: %d %v", n, err)
	}
	var nilClient *KafGraphClient
	nilClient.SyncLoop(context.Background())
	if _, err := nilClient.FetchNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, err := nilClient.SyncOnce(context.Background()); err != nil || n != 0 {
		t.Fatal("nil sync")
	}
	if propStr(map[string]any{"x": 1}, "x") != "" {
		t.Fatal("non-string property accepted")
	}
}

func TestRemoteMemoryFailuresAndLoops(t *testing.T) {
	svc := NewMemoryService(&fakeVectorStore{}, nil)
	for _, mode := range []string{"status", "decode", "offline", "url"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "status" {
					w.WriteHeader(503)
				}
				w.Write([]byte("invalid"))
			}))
			defer server.Close()
			url := server.URL
			if mode == "url" {
				url = ":invalid"
			}
			if mode == "offline" {
				server.Close()
			}
			kg := NewKafGraphClient(KafGraphConfig{URL: url}, svc)
			if _, err := kg.FetchNodes(context.Background()); err == nil {
				t.Fatal("graph fetch failure hidden")
			}
			if _, err := kg.SyncOnce(context.Background()); err == nil {
				t.Fatal("graph sync failure hidden")
			}
			er := NewER1Client(ER1Config{URL: url, GivenName: "Test", FamilyName: "User", Email: "test@example.invalid"}, svc)
			if err := er.Authenticate(context.Background()); err == nil {
				t.Fatal("ER1 auth failure hidden")
			}
			er.ctxID = "ctx"
			if _, err := er.FetchMemories(context.Background()); err == nil {
				t.Fatal("ER1 fetch failure hidden")
			}
			if _, err := er.SyncOnce(context.Background()); err == nil {
				t.Fatal("ER1 sync failure hidden")
			}
			er.SyncLoop(context.Background()) // failed authentication returns immediately
		})
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/access":
			w.Write([]byte(`{"ctx_id":"ctx"}`))
		case "/memory/ctx":
			calls.Add(1)
			w.Write([]byte(`{"memories":[{"id":"1","transcript":"personal note","transcript_status":"processed"},{"id":"pending","transcript_status":"pending"}]}`))
		default:
			calls.Add(1)
			w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()
	er := NewER1Client(ER1Config{URL: server.URL, SyncInterval: time.Millisecond, APIKey: "test"}, svc)
	if er.Status().Connected {
		t.Fatal("connected before auth")
	}
	if err := er.Authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, err := er.SyncOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("ER1 sync: %d %v", n, err)
	}
	er.service = NewMemoryService(&fakeVectorStore{upsertErr: errors.New("failed")}, &fakeEmbedder{})
	if n, err := er.SyncOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("ER1 failed write: %d %v", n, err)
	}
	if !er.Status().Connected || er.Status().LastSync.IsZero() {
		t.Fatal("status not updated")
	}
	for _, run := range []func(context.Context){er.SyncLoop, NewKafGraphClient(KafGraphConfig{URL: server.URL, SyncInterval: time.Millisecond}, svc).SyncLoop} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		run(ctx)
		cancel()
	}
	if calls.Load() < 3 {
		t.Fatal("periodic sync did not run")
	}
	var nilER *ER1Client
	nilER.SyncLoop(context.Background())
	if nilER.Status().Connected {
		t.Fatal("nil connected")
	}
	if _, err := nilER.FetchMemories(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryMaintenanceAndSoulIndexing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteVecStore(db, 2)
	svc := NewMemoryService(store, nil)
	root := t.TempDir()
	t.Setenv("HOME", root)
	if err := os.WriteFile(filepath.Join(root, "SOUL.md"), []byte("# Soul\nIdentity\n## Preferences\nShort answers"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewSoulFileIndexer(svc, "~").IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	lm := NewLifecycleManager(db, LifecycleConfig{})
	stats, err := lm.Stats()
	if err != nil || stats.TotalChunks != 2 {
		t.Fatalf("soul chunks: %+v %v", stats, err)
	}
	if err := NewSoulFileIndexer(svc, root).IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, err := lm.DeleteBySource("soul:"); err != nil || n != 2 {
		t.Fatalf("source deletion: %d %v", n, err)
	}
	if _, err := svc.Store(context.Background(), "note", "user", ""); err != nil {
		t.Fatal(err)
	}
	if n, err := lm.DeleteAll(); err != nil || n != 1 {
		t.Fatalf("delete all: %d %v", n, err)
	}
	if _, err := db.Exec(`CREATE TABLE working_memory (resource_id TEXT NOT NULL, thread_id TEXT NOT NULL DEFAULT '', content TEXT NOT NULL DEFAULT '', updated_at DATETIME, PRIMARY KEY(resource_id,thread_id))`); err != nil {
		t.Fatal(err)
	}
	working := NewWorkingMemoryStore(db)
	if err := working.Save("resource", "thread", "working"); err != nil {
		t.Fatal(err)
	}
	entries, err := working.ListAll()
	if err != nil || len(entries) != 1 || entries[0].Content != "working" {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	if err := working.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if entries, err := working.ListAll(); err != nil || len(entries) != 0 {
		t.Fatal("working memory not cleared")
	}
	if NewWorkingMemoryStore(nil) != nil {
		t.Fatal("nil db accepted")
	}
	if err := NewSoulFileIndexer(NewMemoryService(&fakeVectorStore{upsertErr: errors.New("failed")}, &fakeEmbedder{}), root).IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexer := NewAutoIndexer(svc, AutoIndexerConfig{MinLength: 1, BatchSize: 1, FlushInterval: time.Millisecond, QueueSize: 1})
	indexer.Enqueue(IndexItem{Content: "hello"})
	indexer.Enqueue(IndexItem{Content: "useful note", Source: "test"})
	indexer.Enqueue(IndexItem{Content: "overflow note"})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	go indexer.Run(ctx)
	<-ctx.Done()
	cancel()
	indexer.Stop()
	indexer.indexBatch(context.Background(), []IndexItem{{Content: "direct note"}})
	indexer.service = NewMemoryService(&fakeVectorStore{upsertErr: errors.New("failed")}, &fakeEmbedder{})
	indexer.indexBatch(context.Background(), []IndexItem{{Content: "failed"}})
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	indexer.indexBatch(ctx, []IndexItem{{Content: "cancelled"}})
	db.Close()
	if _, err := lm.DeleteAll(); err == nil {
		t.Fatal("closed db accepted")
	}
	if _, err := lm.DeleteBySource("x"); err == nil {
		t.Fatal("closed db accepted")
	}
	lm.RunDaily()
	if _, err := working.ListAll(); err == nil {
		t.Fatal("closed db accepted")
	}
	if err := working.DeleteAll(); err == nil {
		t.Fatal("closed db accepted")
	}
	var nilLM *LifecycleManager
	if n, err := nilLM.DeleteAll(); err != nil || n != 0 {
		t.Fatal("nil delete")
	}
	if n, err := nilLM.DeleteBySource("x"); err != nil || n != 0 {
		t.Fatal("nil delete")
	}
	var nilWorking *WorkingMemoryStore
	if _, err := nilWorking.ListAll(); err != nil {
		t.Fatal(err)
	}
	if err := nilWorking.DeleteAll(); err != nil {
		t.Fatal(err)
	}
}
