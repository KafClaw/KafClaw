package provider

import (
	"context"
)

const xaiDefaultBase = "https://api.x.ai/v1"

// XAIProvider wraps OpenAIProvider with the xAI/Grok API base URL.
type XAIProvider struct {
	inner *OpenAIProvider
}

// NewXAIProvider creates a provider targeting the xAI API.
func NewXAIProvider(apiKey, defaultModel string) *XAIProvider {
	if defaultModel == "" {
		defaultModel = "grok-3"
	}
	return &XAIProvider{
		inner: NewOpenAIProvider(apiKey, xaiDefaultBase, defaultModel),
	}
}

func (p *XAIProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return p.inner.Chat(ctx, req)
}

func (p *XAIProvider) Transcribe(ctx context.Context, req *AudioRequest) (*AudioResponse, error) {
	return p.inner.Transcribe(ctx, req)
}

func (p *XAIProvider) Speak(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	return p.inner.Speak(ctx, req)
}

func (p *XAIProvider) DefaultModel() string {
	return p.inner.DefaultModel()
}
