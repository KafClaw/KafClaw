package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProvider_DefaultModel(t *testing.T) {
	p := NewOpenAIProvider("test-key", "", "")
	if p.DefaultModel() != "anthropic/claude-sonnet-4-5" {
		t.Errorf("expected default model anthropic/claude-sonnet-4-5, got %s", p.DefaultModel())
	}

	p = NewOpenAIProvider("test-key", "", "openai/gpt-4")
	if p.DefaultModel() != "openai/gpt-4" {
		t.Errorf("expected model openai/gpt-4, got %s", p.DefaultModel())
	}
}

func TestOpenAIProvider_ParseSimpleResponse(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIResponse{
			Choices: []openAIChoice{
				{
					Message:      openAIMessage{Role: "assistant", Content: "Hello, world!"},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL, "test-model")
	resp, err := p.Chat(context.Background(), &ChatRequest{
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		MaxTokens:   100,
		Temperature: 0.7,
	})

	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if resp.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got '%s'", resp.Content)
	}

	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.FinishReason)
	}

	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected total_tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAIProvider_ParseToolCallResponse(t *testing.T) {
	// Mock server with tool call response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIResponse{
			Choices: []openAIChoice{
				{
					Message: openAIMessage{
						Role:    "assistant",
						Content: "",
						ToolCalls: []openAIToolCall{
							{
								ID:   "call_123",
								Type: "function",
								Function: struct {
									Name      string `json:"name"`
									Arguments string `json:"arguments"`
								}{
									Name:      "read_file",
									Arguments: `{"path": "/tmp/test.txt"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL, "test-model")
	resp, err := p.Chat(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Read the file"}},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDef{
					Name:        "read_file",
					Description: "Read a file",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	})

	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got '%s'", tc.Name)
	}

	if tc.Arguments["path"] != "/tmp/test.txt" {
		t.Errorf("expected path '/tmp/test.txt', got '%v'", tc.Arguments["path"])
	}
}

func TestOpenAIProvider_APIError(t *testing.T) {
	// Mock server returning error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid API key"}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider("bad-key", server.URL, "test-model")
	_, err := p.Chat(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})

	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}
