package middleware

import (
	"context"
	"testing"

	"github.com/KafClaw/KafClaw/internal/provider"
)

// mockProvider is a simple test provider.
type mockProvider struct {
	response *provider.ChatResponse
	err      error
	called   bool
}

func (m *mockProvider) Chat(_ context.Context, _ *provider.ChatRequest) (*provider.ChatResponse, error) {
	m.called = true
	return m.response, m.err
}

func (m *mockProvider) Transcribe(_ context.Context, _ *provider.AudioRequest) (*provider.AudioResponse, error) {
	return nil, nil
}

func (m *mockProvider) Speak(_ context.Context, _ *provider.TTSRequest) (*provider.TTSResponse, error) {
	return nil, nil
}

func (m *mockProvider) DefaultModel() string { return "mock-model" }

// noopMiddleware does nothing.
type noopMiddleware struct{}

func (n *noopMiddleware) Name() string { return "noop" }
func (n *noopMiddleware) ProcessRequest(_ context.Context, _ *provider.ChatRequest, _ *RequestMeta) error {
	return nil
}
func (n *noopMiddleware) ProcessResponse(_ context.Context, _ *provider.ChatRequest, _ *provider.ChatResponse, _ *RequestMeta) error {
	return nil
}

// tagMiddleware sets a tag in ProcessRequest and reads response in ProcessResponse.
type tagMiddleware struct {
	tagKey   string
	tagValue string
	postSeen bool
}

func (t *tagMiddleware) Name() string { return "tagger" }
func (t *tagMiddleware) ProcessRequest(_ context.Context, _ *provider.ChatRequest, meta *RequestMeta) error {
	meta.Tags[t.tagKey] = t.tagValue
	return nil
}
func (t *tagMiddleware) ProcessResponse(_ context.Context, _ *provider.ChatRequest, _ *provider.ChatResponse, _ *RequestMeta) error {
	t.postSeen = true
	return nil
}

// blockMiddleware blocks requests.
type blockMiddleware struct{}

func (b *blockMiddleware) Name() string { return "blocker" }
func (b *blockMiddleware) ProcessRequest(_ context.Context, _ *provider.ChatRequest, meta *RequestMeta) error {
	meta.Blocked = true
	meta.BlockReason = "test block"
	return nil
}
func (b *blockMiddleware) ProcessResponse(_ context.Context, _ *provider.ChatRequest, _ *provider.ChatResponse, _ *RequestMeta) error {
	return nil
}

func TestChain_Passthrough(t *testing.T) {
	mp := &mockProvider{response: &provider.ChatResponse{Content: "hello"}}
	chain := NewChain(mp)

	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	resp, err := chain.Process(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected 'hello', got %q", resp.Content)
	}
	if !mp.called {
		t.Error("expected provider to be called")
	}
}

func TestChain_NoopMiddleware(t *testing.T) {
	mp := &mockProvider{response: &provider.ChatResponse{Content: "hello"}}
	chain := NewChain(mp)
	chain.Use(&noopMiddleware{})

	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	resp, err := chain.Process(context.Background(), req, NewRequestMeta("test", "model"))
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected 'hello', got %q", resp.Content)
	}
}

func TestChain_TagMiddleware(t *testing.T) {
	mp := &mockProvider{response: &provider.ChatResponse{Content: "world"}}
	chain := NewChain(mp)
	tagger := &tagMiddleware{tagKey: "task", tagValue: "coding"}
	chain.Use(tagger)

	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "code"}}}
	resp, err := chain.Process(context.Background(), req, meta)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if resp.Content != "world" {
		t.Errorf("expected 'world', got %q", resp.Content)
	}
	if meta.Tags["task"] != "coding" {
		t.Errorf("expected tag 'coding', got %q", meta.Tags["task"])
	}
	if !tagger.postSeen {
		t.Error("expected post-hook to run")
	}
}

func TestChain_BlockedRequest(t *testing.T) {
	mp := &mockProvider{response: &provider.ChatResponse{Content: "should not see"}}
	chain := NewChain(mp)
	chain.Use(&blockMiddleware{})

	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "blocked content"}}}
	resp, err := chain.Process(context.Background(), req, meta)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if resp.FinishReason != "blocked" {
		t.Errorf("expected finish_reason 'blocked', got %q", resp.FinishReason)
	}
	if mp.called {
		t.Error("expected provider NOT to be called when blocked")
	}
}

func TestChain_ProviderOverride(t *testing.T) {
	mp1 := &mockProvider{response: &provider.ChatResponse{Content: "from mp1"}}
	mp2 := &mockProvider{response: &provider.ChatResponse{Content: "from mp2"}}

	// Middleware that swaps the provider
	swapper := &providerSwapMiddleware{replacement: mp2}
	chain := NewChain(mp1)
	chain.Use(swapper)

	meta := NewRequestMeta("original", "model")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	resp, err := chain.Process(context.Background(), req, meta)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if resp.Content != "from mp2" {
		t.Errorf("expected 'from mp2', got %q", resp.Content)
	}
	if mp1.called {
		t.Error("expected original provider NOT to be called")
	}
	if !mp2.called {
		t.Error("expected replacement provider to be called")
	}
}

type providerSwapMiddleware struct {
	replacement provider.LLMProvider
}

func (p *providerSwapMiddleware) Name() string { return "swapper" }
func (p *providerSwapMiddleware) ProcessRequest(_ context.Context, _ *provider.ChatRequest, meta *RequestMeta) error {
	meta.ProviderOverride = p.replacement
	return nil
}
func (p *providerSwapMiddleware) ProcessResponse(_ context.Context, _ *provider.ChatRequest, _ *provider.ChatResponse, _ *RequestMeta) error {
	return nil
}

func TestChain_MultipleMiddlewareOrdering(t *testing.T) {
	mp := &mockProvider{response: &provider.ChatResponse{Content: "ok"}}
	chain := NewChain(mp)

	order := make([]string, 0, 4)
	chain.Use(
		&orderTracker{name: "first", order: &order},
		&orderTracker{name: "second", order: &order},
	)

	meta := NewRequestMeta("test", "model")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	_, err := chain.Process(context.Background(), req, meta)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	expected := []string{"first-pre", "second-pre", "first-post", "second-post"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("event[%d] = %q, want %q", i, order[i], v)
		}
	}
}

type orderTracker struct {
	name  string
	order *[]string
}

func (o *orderTracker) Name() string { return o.name }
func (o *orderTracker) ProcessRequest(_ context.Context, _ *provider.ChatRequest, _ *RequestMeta) error {
	*o.order = append(*o.order, o.name+"-pre")
	return nil
}
func (o *orderTracker) ProcessResponse(_ context.Context, _ *provider.ChatRequest, _ *provider.ChatResponse, _ *RequestMeta) error {
	*o.order = append(*o.order, o.name+"-post")
	return nil
}

func TestNewRequestMeta(t *testing.T) {
	meta := NewRequestMeta("openai", "gpt-4")
	if meta.ProviderID != "openai" {
		t.Errorf("expected provider 'openai', got %q", meta.ProviderID)
	}
	if meta.ModelName != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", meta.ModelName)
	}
	if meta.Tags == nil {
		t.Error("expected non-nil Tags map")
	}
}
