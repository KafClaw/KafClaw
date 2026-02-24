package cli

import (
	"context"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

type testLLMOnly struct{}

func (t *testLLMOnly) Chat(context.Context, *provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{}, nil
}
func (t *testLLMOnly) Transcribe(context.Context, *provider.AudioRequest) (*provider.AudioResponse, error) {
	return &provider.AudioResponse{}, nil
}
func (t *testLLMOnly) Speak(context.Context, *provider.TTSRequest) (*provider.TTSResponse, error) {
	return &provider.TTSResponse{}, nil
}
func (t *testLLMOnly) DefaultModel() string { return "x" }

type testEmbedderLLM struct {
	testLLMOnly
	lastReq *provider.EmbeddingRequest
}

func (t *testEmbedderLLM) Embed(_ context.Context, req *provider.EmbeddingRequest) (*provider.EmbeddingResponse, error) {
	if req != nil {
		cp := *req
		t.lastReq = &cp
	}
	return &provider.EmbeddingResponse{Vector: []float32{1, 2, 3}}, nil
}

func TestResolveMemoryEmbedder_Disabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = false

	emb, src := resolveMemoryEmbedder(cfg, &testLLMOnly{})
	if emb != nil {
		t.Fatalf("expected nil embedder when disabled")
	}
	if src != "disabled by config" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestResolveMemoryEmbedder_OpenAI(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = "openai"
	cfg.Providers.OpenAI.APIKey = "k"

	emb, src := resolveMemoryEmbedder(cfg, &testLLMOnly{})
	if emb == nil {
		t.Fatalf("expected embedder for openai")
	}
	if src != "openai" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestResolveMemoryEmbedder_LocalHFUsesMainProviderEmbedder(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = "local-hf"
	cfg.Memory.Embedding.Model = "custom-model"
	main := &testEmbedderLLM{}

	emb, src := resolveMemoryEmbedder(cfg, main)
	if emb == nil {
		t.Fatalf("expected embedder from main provider")
	}
	if src != "main-provider" {
		t.Fatalf("unexpected source: %s", src)
	}
	_, _ = emb.Embed(context.Background(), &provider.EmbeddingRequest{Input: "hello"})
	if main.lastReq == nil || main.lastReq.Model != "custom-model" {
		t.Fatalf("expected default model pinning, got %+v", main.lastReq)
	}
}

func TestResolveMemoryEmbedder_UnknownProviderFallsBackToOpenAIKey(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = "x-unknown"
	cfg.Providers.OpenAI.APIKey = "k"

	emb, src := resolveMemoryEmbedder(cfg, &testLLMOnly{})
	if emb == nil {
		t.Fatalf("expected fallback openai embedder")
	}
	if src != "openai-fallback" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestResolveMemoryEmbedder_UnknownProviderWithoutFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = "x-unknown"
	cfg.Providers.OpenAI.APIKey = ""

	emb, src := resolveMemoryEmbedder(cfg, &testLLMOnly{})
	if emb != nil {
		t.Fatalf("expected nil embedder when unsupported and no fallback")
	}
	if src != "unsupported embedding provider" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestResolveMemoryEmbedder_NilConfig(t *testing.T) {
	emb, src := resolveMemoryEmbedder(nil, &testLLMOnly{})
	if emb != nil {
		t.Fatalf("expected nil embedder")
	}
	if src != "config unavailable" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestDefaultModelEmbedder_PreservesExplicitModel(t *testing.T) {
	main := &testEmbedderLLM{}
	wrapped := withDefaultEmbeddingModel(main, "default-model")

	_, _ = wrapped.Embed(context.Background(), &provider.EmbeddingRequest{Input: "x", Model: "explicit"})
	if main.lastReq == nil || main.lastReq.Model != "explicit" {
		t.Fatalf("expected explicit model preserved, got %+v", main.lastReq)
	}
}

func TestResolveMemoryEmbedder_AutoFallbackNoEmbedder(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = "auto"
	cfg.Providers.OpenAI.APIKey = ""

	emb, src := resolveMemoryEmbedder(cfg, &testLLMOnly{})
	if emb != nil {
		t.Fatalf("expected nil embedder when no source available")
	}
	if src != "no embedder available" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestResolveMemoryEmbedder_AutoFallsBackToOpenAIWhenKeyPresent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = ""
	cfg.Providers.OpenAI.APIKey = "k"

	emb, src := resolveMemoryEmbedder(cfg, &testLLMOnly{})
	if emb == nil {
		t.Fatalf("expected openai fallback embedder")
	}
	if src != "openai-fallback" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestResolveMemoryEmbedder_LocalHFFallsBackToOpenAIWhenMainMissing(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = "local-hf"
	cfg.Providers.OpenAI.APIKey = "k"

	emb, src := resolveMemoryEmbedder(cfg, &testLLMOnly{})
	if emb == nil {
		t.Fatalf("expected openai fallback embedder")
	}
	if src != "openai-fallback" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestResolveMemoryEmbedder_UnknownProviderUsesMainEmbedderFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Memory.Embedding.Enabled = true
	cfg.Memory.Embedding.Provider = "something-custom"
	main := &testEmbedderLLM{}

	emb, src := resolveMemoryEmbedder(cfg, main)
	if emb == nil {
		t.Fatalf("expected main-provider-fallback embedder")
	}
	if src != "main-provider-fallback" {
		t.Fatalf("unexpected source: %s", src)
	}
}

func TestWithDefaultEmbeddingModel_NoWrapCases(t *testing.T) {
	main := &testEmbedderLLM{}
	if got := withDefaultEmbeddingModel(nil, "x"); got != nil {
		t.Fatalf("expected nil passthrough for nil inner")
	}
	if got := withDefaultEmbeddingModel(main, "   "); got != main {
		t.Fatalf("expected passthrough when model is empty")
	}
}

func TestDefaultModelEmbedder_NilRequest(t *testing.T) {
	main := &testEmbedderLLM{}
	wrapped := withDefaultEmbeddingModel(main, "default-model")

	_, _ = wrapped.Embed(context.Background(), nil)
	if main.lastReq == nil || main.lastReq.Model != "default-model" {
		t.Fatalf("expected default model injected for nil req, got %+v", main.lastReq)
	}
}
