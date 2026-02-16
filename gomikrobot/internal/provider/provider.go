// Package provider implements LLM provider interfaces and clients.
package provider

import (
	"context"
)

// LLMProvider is the interface for LLM API clients.
type LLMProvider interface {
	// Chat sends a completion request and returns the response.
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	// Transcribe converts audio to text.
	Transcribe(ctx context.Context, req *AudioRequest) (*AudioResponse, error)
	// Speak converts text to audio.
	Speak(ctx context.Context, req *TTSRequest) (*TTSResponse, error)
	// DefaultModel returns the configured default model.
	DefaultModel() string
}

// TTSRequest contains parameters for speech synthesis.
type TTSRequest struct {
	Text  string
	Voice string
	Speed float64
}

// TTSResponse contains the synthesized audio.
type TTSResponse struct {
	AudioData []byte
	Format    string
}

// AudioRequest contains parameters for transcription.
type AudioRequest struct {
	FilePath string
	Model    string
}

// AudioResponse contains the transcribed text.
type AudioResponse struct {
	Text string
}

// ChatRequest contains the parameters for a chat completion request.
type ChatRequest struct {
	Messages    []Message
	Tools       []ToolDefinition
	Model       string
	MaxTokens   int
	Temperature float64
}

// ChatResponse contains the response from a chat completion request.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool call from the LLM.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolDefinition defines a tool that can be called by the LLM.
type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a function that can be called.
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Usage contains token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Embedder is an optional interface for providers that support embedding.
// Not all providers implement this (e.g. LocalWhisperProvider does not).
// Callers should use type assertion: if emb, ok := prov.(Embedder); ok { ... }
type Embedder interface {
	Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)
}

// EmbeddingRequest contains parameters for an embedding request.
type EmbeddingRequest struct {
	Input string
	Model string // default: "text-embedding-3-small"
}

// EmbeddingResponse contains the embedding vector.
type EmbeddingResponse struct {
	Vector []float32
	Usage  Usage
}
