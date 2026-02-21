// Package middleware provides a chain of interceptors between the agent loop
// and the LLM provider. Middleware can inspect, transform, reroute, or filter
// messages before and after the LLM call.
package middleware

import (
	"context"
	"fmt"

	"github.com/KafClaw/KafClaw/internal/provider"
)

// ChatMiddleware intercepts LLM requests and/or responses.
type ChatMiddleware interface {
	// Name returns a short identifier for logging/metrics.
	Name() string
	// ProcessRequest is called before the LLM call. It may modify the request,
	// replace the provider (by setting meta.ProviderOverride), or return an
	// error to abort.
	ProcessRequest(ctx context.Context, req *provider.ChatRequest, meta *RequestMeta) error
	// ProcessResponse is called after the LLM call. It may modify the response
	// or return an error to suppress delivery.
	ProcessResponse(ctx context.Context, req *provider.ChatRequest, resp *provider.ChatResponse, meta *RequestMeta) error
}

// RequestMeta carries mutable context through the chain.
type RequestMeta struct {
	ProviderID       string               // resolved provider; middleware can override
	ModelName        string               // resolved model; middleware can override
	SenderID         string               // who sent the message
	Channel          string               // e.g. "telegram", "discord", "cli"
	MessageType      string               // "internal" / "external"
	Tags             map[string]string    // classification tags (e.g. "sensitivity":"pii", "task":"coding")
	Blocked          bool                 // set by PromptGuard to abort
	BlockReason      string               // reason for blocking
	ProviderOverride provider.LLMProvider // middleware can swap the provider
	CostUSD          float64              // set by FinOps recorder
}

// NewRequestMeta creates a RequestMeta with initialized Tags map.
func NewRequestMeta(providerID, modelName string) *RequestMeta {
	return &RequestMeta{
		ProviderID: providerID,
		ModelName:  modelName,
		Tags:       make(map[string]string),
	}
}

// Chain holds an ordered list of middleware and a default provider.
// It runs pre-hooks in order, calls the provider, then runs post-hooks in order.
type Chain struct {
	Middlewares []ChatMiddleware
	Provider    provider.LLMProvider
}

// NewChain creates a chain with the given provider and no middleware.
func NewChain(prov provider.LLMProvider) *Chain {
	return &Chain{
		Provider: prov,
	}
}

// Use appends middleware to the chain.
func (c *Chain) Use(mw ...ChatMiddleware) {
	c.Middlewares = append(c.Middlewares, mw...)
}

// Process runs the middleware chain: pre-hooks → LLM call → post-hooks.
// If no middleware is configured, it's a zero-overhead passthrough.
func (c *Chain) Process(ctx context.Context, req *provider.ChatRequest, meta *RequestMeta) (*provider.ChatResponse, error) {
	if meta == nil {
		meta = NewRequestMeta("", "")
	}

	// Run pre-hooks.
	for _, mw := range c.Middlewares {
		if err := mw.ProcessRequest(ctx, req, meta); err != nil {
			return nil, fmt.Errorf("middleware %s pre-hook: %w", mw.Name(), err)
		}
		if meta.Blocked {
			return &provider.ChatResponse{
				Content:      fmt.Sprintf("[blocked by %s] %s", mw.Name(), meta.BlockReason),
				FinishReason: "blocked",
			}, nil
		}
	}

	// Determine the provider to use.
	prov := c.Provider
	if meta.ProviderOverride != nil {
		prov = meta.ProviderOverride
	}

	// Make the LLM call.
	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// Run post-hooks.
	for _, mw := range c.Middlewares {
		if err := mw.ProcessResponse(ctx, req, resp, meta); err != nil {
			return nil, fmt.Errorf("middleware %s post-hook: %w", mw.Name(), err)
		}
	}

	return resp, nil
}
