package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/KafClaw/KafClaw/internal/provider/clicache"
)

const geminiDefaultBase = "https://generativelanguage.googleapis.com/v1beta"

// GeminiProvider implements LLMProvider using the Gemini REST API.
// It supports two auth modes:
//   - Static API key (query param) for provider ID "gemini"
//   - OAuth bearer from CLI cache for provider ID "gemini-cli"
type GeminiProvider struct {
	apiKey       string // empty for OAuth mode
	useOAuth     bool
	defaultModel string
	httpClient   *http.Client
}

// NewGeminiProvider creates a Gemini provider using a static API key.
func NewGeminiProvider(apiKey, defaultModel string) *GeminiProvider {
	if defaultModel == "" {
		defaultModel = "gemini-2.0-flash"
	}
	return &GeminiProvider{
		apiKey:       apiKey,
		useOAuth:     false,
		defaultModel: defaultModel,
		httpClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

// NewGeminiCLIProvider creates a Gemini provider using OAuth credentials from the Gemini CLI.
func NewGeminiCLIProvider(defaultModel string) *GeminiProvider {
	if defaultModel == "" {
		defaultModel = "gemini-2.0-flash"
	}
	return &GeminiProvider{
		useOAuth:     true,
		defaultModel: defaultModel,
		httpClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *GeminiProvider) DefaultModel() string {
	return p.defaultModel
}

func (p *GeminiProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	gemReq := p.buildGeminiRequest(req)
	jsonBody, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", geminiDefaultBase, model)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := p.setAuth(httpReq); err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return p.parseGeminiResponse(respBody)
}

func (p *GeminiProvider) Transcribe(_ context.Context, _ *AudioRequest) (*AudioResponse, error) {
	return nil, fmt.Errorf("gemini provider does not support transcription")
}

func (p *GeminiProvider) Speak(_ context.Context, _ *TTSRequest) (*TTSResponse, error) {
	return nil, fmt.Errorf("gemini provider does not support TTS")
}

func (p *GeminiProvider) setAuth(req *http.Request) error {
	if p.useOAuth {
		tok, err := clicache.ReadGeminiCLICredential()
		if err != nil {
			return fmt.Errorf("gemini-cli auth: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok.Access)
	} else {
		q := req.URL.Query()
		q.Set("key", p.apiKey)
		req.URL.RawQuery = q.Encode()
	}
	return nil
}

// --- Gemini request/response types ---

type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	Tools            []geminiTool            `json:"tools,omitempty"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text         string                  `json:"text,omitempty"`
	FunctionCall *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResp *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func (p *GeminiProvider) buildGeminiRequest(req *ChatRequest) *geminiRequest {
	gemReq := &geminiRequest{
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		},
	}

	for _, msg := range req.Messages {
		role := msg.Role
		switch role {
		case "assistant":
			role = "model"
		case "system":
			role = "user"
		}

		content := geminiContent{Role: role}

		if msg.Content != "" {
			content.Parts = append(content.Parts, geminiPart{Text: msg.Content})
		}

		// Convert tool calls from assistant messages.
		for _, tc := range msg.ToolCalls {
			content.Parts = append(content.Parts, geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: tc.Name,
					Args: tc.Arguments,
				},
			})
		}

		// Convert tool responses.
		if msg.ToolCallID != "" && msg.Role == "tool" {
			content.Role = "function"
			content.Parts = []geminiPart{{
				FunctionResp: &geminiFunctionResponse{
					Name:     msg.ToolCallID,
					Response: map[string]any{"result": msg.Content},
				},
			}}
		}

		gemReq.Contents = append(gemReq.Contents, content)
	}

	if len(req.Tools) > 0 {
		var decls []geminiFunctionDecl
		for _, t := range req.Tools {
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		gemReq.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}

	return gemReq
}

func (p *GeminiProvider) parseGeminiResponse(body []byte) (*ChatResponse, error) {
	var gemResp geminiResponse
	if err := json.Unmarshal(body, &gemResp); err != nil {
		return nil, fmt.Errorf("parse gemini response: %w", err)
	}

	if len(gemResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in gemini response")
	}

	candidate := gemResp.Candidates[0]
	result := &ChatResponse{
		FinishReason: candidate.FinishReason,
	}

	// Extract usage.
	if gemResp.UsageMetadata != nil {
		result.Usage = Usage{
			PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gemResp.UsageMetadata.TotalTokenCount,
		}
	}

	// Extract text and tool calls from parts.
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			result.Content += part.Text
		}
		if part.FunctionCall != nil {
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        part.FunctionCall.Name,
				Name:      part.FunctionCall.Name,
				Arguments: part.FunctionCall.Args,
			})
		}
	}

	return result, nil
}
