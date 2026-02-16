package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenAIProvider implements LLMProvider using the OpenAI-compatible API.
// It supports OpenRouter, Anthropic, OpenAI, and other compatible providers.
type OpenAIProvider struct {
	apiKey       string
	apiBase      string
	defaultModel string
	httpClient   *http.Client
}

// NewOpenAIProvider creates a new OpenAI-compatible provider.
func NewOpenAIProvider(apiKey, apiBase, defaultModel string) *OpenAIProvider {
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	if defaultModel == "" {
		defaultModel = "anthropic/claude-sonnet-4-5"
	}
	return &OpenAIProvider{
		apiKey:       apiKey,
		apiBase:      strings.TrimSuffix(apiBase, "/"),
		defaultModel: defaultModel,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultModel returns the configured default model.
func (p *OpenAIProvider) DefaultModel() string {
	return p.defaultModel
}

// Chat sends a completion request to the OpenAI-compatible API.
func (p *OpenAIProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	// Build request body
	body := map[string]any{
		"model":       model,
		"messages":    p.convertMessages(req.Messages),
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
	}

	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		body["tool_choice"] = "auto"
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var apiResp openAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return p.parseResponse(&apiResp)
}

// convertMessages converts our Message type to OpenAI API format.
func (p *OpenAIProvider) convertMessages(messages []Message) []map[string]any {
	result := make([]map[string]any, len(messages))
	for i, msg := range messages {
		m := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.ToolCallID != "" {
			m["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]map[string]any, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				args, _ := json.Marshal(tc.Arguments)
				toolCalls[j] = map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": string(args),
					},
				}
			}
			m["tool_calls"] = toolCalls
		}
		result[i] = m
	}
	return result
}

// parseResponse converts the API response to our ChatResponse type.
func (p *OpenAIProvider) parseResponse(resp *openAIResponse) (*ChatResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	result := &ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// Parse tool calls
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]any{"raw": tc.Function.Arguments}
			}
		}
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return result, nil
}

// OpenAI API response types
type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Transcribe converts audio to text using OpenAI Whisper API.
func (p *OpenAIProvider) Transcribe(ctx context.Context, req *AudioRequest) (*AudioResponse, error) {
	model := req.Model
	if model == "" {
		model = "whisper-1"
	}

	file, err := os.Open(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("open audio file: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(req.FilePath))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("copy file to form: %w", err)
	}

	_ = writer.WriteField("model", model)
	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("close form writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/audio/transcriptions", body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Whisper API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var audioResp struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &audioResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &AudioResponse{Text: audioResp.Text}, nil
}

// Embed generates an embedding vector for the given input text.
func (p *OpenAIProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	model := req.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	body := map[string]any{
		"model": model,
		"input": req.Input,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute embedding request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var embResp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("parse embedding response: %w", err)
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data in response")
	}

	return &EmbeddingResponse{
		Vector: embResp.Data[0].Embedding,
		Usage: Usage{
			PromptTokens: embResp.Usage.PromptTokens,
			TotalTokens:  embResp.Usage.TotalTokens,
		},
	}, nil
}

// Speak converts text to audio using OpenAI TTS API.
func (p *OpenAIProvider) Speak(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	if req.Voice == "" {
		req.Voice = "nova" // Default to a neutral, professional voice
	}
	model := "tts-1"

	reqBody := map[string]interface{}{
		"model":           model,
		"input":           req.Text,
		"voice":           req.Voice,
		"response_format": "opus", // Get Opus directly for WhatsApp
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/audio/speech", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return &TTSResponse{
		AudioData: audioData,
		Format:    "opus",
	}, nil
}
