package provider

import (
	"context"
	"fmt"

	"github.com/KafClaw/KafClaw/internal/provider/clicache"
)

// CodexProvider wraps OpenAIProvider with OAuth bearer token from the Codex CLI cache.
type CodexProvider struct {
	defaultModel string
}

// NewCodexProvider creates a provider that uses Codex CLI OAuth credentials.
func NewCodexProvider(defaultModel string) *CodexProvider {
	if defaultModel == "" {
		defaultModel = "gpt-5.3-codex"
	}
	return &CodexProvider{defaultModel: defaultModel}
}

func (p *CodexProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	inner, err := p.resolveInner()
	if err != nil {
		return nil, err
	}
	return inner.Chat(ctx, req)
}

func (p *CodexProvider) Transcribe(ctx context.Context, req *AudioRequest) (*AudioResponse, error) {
	inner, err := p.resolveInner()
	if err != nil {
		return nil, err
	}
	return inner.Transcribe(ctx, req)
}

func (p *CodexProvider) Speak(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	inner, err := p.resolveInner()
	if err != nil {
		return nil, err
	}
	return inner.Speak(ctx, req)
}

func (p *CodexProvider) DefaultModel() string {
	return p.defaultModel
}

func (p *CodexProvider) resolveInner() (*OpenAIProvider, error) {
	tok, err := clicache.ReadCodexCLICredential()
	if err != nil {
		return nil, fmt.Errorf("codex provider: %w", err)
	}
	return NewOpenAIProvider(tok.Access, "https://api.openai.com/v1", p.defaultModel), nil
}
