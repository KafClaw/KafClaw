package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

type contractTransport func(*http.Request) (*http.Response, error)

func (f contractTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("body failed") }
func (brokenBody) Close() error             { return nil }

func TestOpenAIEndpoints(t *testing.T) {
	file := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(file, []byte("audio fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	p := NewOpenAIProvider("test-key", "https://example.invalid/v1/", "chat-model")
	p.httpClient = &http.Client{Transport: contractTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("invalid method/auth: %s", r.Method)
		}
		var body map[string]any
		response := ""
		switch r.URL.Path {
		case "/v1/audio/transcriptions":
			if err := r.ParseMultipartForm(1024); err != nil {
				t.Fatal(err)
			}
			model := r.FormValue("model")
			if model != "whisper-1" && model != "custom-whisper" {
				t.Errorf("transcription model: %s", model)
			}
			f, h, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			if h.Filename != "audio.wav" || string(b) != "audio fixture" {
				t.Error("audio multipart payload changed")
			}
			response = `{"text":"transcribed"}`
		case "/v1/embeddings":
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["input"] != "embed me" {
				t.Errorf("embedding input: %v", body)
			}
			if body["model"] != "text-embedding-3-small" && body["model"] != "custom-embedding" {
				t.Errorf("embedding model: %v", body)
			}
			response = `{"data":[{"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":3,"total_tokens":3}}`
		case "/v1/audio/speech":
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["input"] != "speak me" || body["response_format"] != "opus" || body["model"] != "tts-1" {
				t.Errorf("speech body: %v", body)
			}
			if body["voice"] != "nova" && body["voice"] != "custom-voice" {
				t.Errorf("voice: %v", body)
			}
			response = "opus fixture"
		default:
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
	})}
	for _, model := range []string{"", "custom-whisper"} {
		got, err := p.Transcribe(context.Background(), &AudioRequest{FilePath: file, Model: model})
		if err != nil || got.Text != "transcribed" {
			t.Fatalf("transcription: %+v %v", got, err)
		}
	}
	for _, model := range []string{"", "custom-embedding"} {
		got, err := p.Embed(context.Background(), &EmbeddingRequest{Input: "embed me", Model: model})
		if err != nil || !reflect.DeepEqual(got.Vector, []float32{.1, .2}) || got.Usage.TotalTokens != 3 {
			t.Fatalf("embedding: %+v %v", got, err)
		}
	}
	for _, voice := range []string{"", "custom-voice"} {
		got, err := p.Speak(context.Background(), &TTSRequest{Text: "speak me", Voice: voice})
		if err != nil || string(got.AudioData) != "opus fixture" || got.Format != "opus" {
			t.Fatalf("speech: %+v %v", got, err)
		}
	}
}

func TestOpenAIEndpointFailures(t *testing.T) {
	file := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	operations := map[string]func(*OpenAIProvider) error{
		"chat": func(p *OpenAIProvider) error { _, err := p.Chat(context.Background(), &ChatRequest{}); return err },
		"transcribe": func(p *OpenAIProvider) error {
			_, err := p.Transcribe(context.Background(), &AudioRequest{FilePath: file})
			return err
		},
		"embed": func(p *OpenAIProvider) error {
			_, err := p.Embed(context.Background(), &EmbeddingRequest{})
			return err
		},
		"speak": func(p *OpenAIProvider) error { _, err := p.Speak(context.Background(), &TTSRequest{}); return err },
	}
	for name, call := range operations {
		t.Run(name, func(t *testing.T) {
			for _, mode := range []string{"transport", "read", "status", "url"} {
				t.Run(mode, func(t *testing.T) {
					p := NewOpenAIProvider("test", "https://example.invalid", "")
					if mode == "url" {
						p.apiBase = ":invalid"
					}
					p.httpClient = &http.Client{Transport: contractTransport(func(r *http.Request) (*http.Response, error) {
						if mode == "transport" {
							return nil, errors.New("offline")
						}
						status := 200
						var body io.ReadCloser = brokenBody{}
						if mode == "status" {
							status = 429
							body = io.NopCloser(strings.NewReader("rate limited"))
						}
						return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}, nil
					})}
					if err := call(p); err == nil {
						t.Fatal("expected failure")
					}
				})
			}
			if name != "speak" {
				for _, body := range []string{"invalid", "{}"} {
					p := NewOpenAIProvider("test", "https://example.invalid", "")
					p.httpClient = &http.Client{Transport: contractTransport(func(r *http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
					})}
					err := call(p)
					if body == "{}" && name == "transcribe" {
						if err != nil {
							t.Fatal(err)
						}
					} else if err == nil {
						t.Fatal("invalid response accepted")
					}
				}
			}
		})
	}
	p := NewOpenAIProvider("test", "", "")
	if _, err := p.Transcribe(context.Background(), &AudioRequest{FilePath: "/nonexistent/audio.wav"}); err == nil {
		t.Fatal("missing audio accepted")
	}
	if _, err := p.Transcribe(context.Background(), &AudioRequest{FilePath: t.TempDir()}); err == nil {
		t.Fatal("directory audio accepted")
	}
	if _, err := p.Chat(context.Background(), &ChatRequest{Tools: []ToolDefinition{{Function: FunctionDef{Parameters: map[string]any{"invalid": make(chan int)}}}}}); err == nil {
		t.Fatal("unserializable tool accepted")
	}
	messages := p.convertMessages([]Message{{Role: "assistant", ToolCalls: []ToolCall{{ID: "id", Name: "tool", Arguments: map[string]any{"x": 1}}}}, {Role: "tool", ToolCallID: "id", Content: "result"}})
	if messages[1]["tool_call_id"] != "id" || len(messages[0]["tool_calls"].([]map[string]any)) != 1 {
		t.Fatal("tool context lost")
	}
	var raw openAIResponse
	if err := json.Unmarshal([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"id","function":{"name":"tool","arguments":"broken"}}]}}]}`), &raw); err != nil {
		t.Fatal(err)
	}
	got, err := p.parseResponse(&raw)
	if err != nil || got.ToolCalls[0].Arguments["raw"] != "broken" {
		t.Fatalf("malformed arguments fallback: %+v %v", got, err)
	}
}

func TestGeminiChatContract(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		t.Run(map[bool]string{false: "api-key", true: "oauth"}[oauth], func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			if err := os.MkdirAll(filepath.Join(root, ".gemini"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".gemini/oauth_creds.json"), []byte(`{"access_token":"oauth-test","expires_at_seconds":4102444800}`), 0600); err != nil {
				t.Fatal(err)
			}
			p := NewGeminiProvider("key-test", "")
			if oauth {
				p = NewGeminiCLIProvider("")
			}
			if p.DefaultModel() != "gemini-2.0-flash" {
				t.Fatal("default model changed")
			}
			p.httpClient = &http.Client{Transport: contractTransport(func(r *http.Request) (*http.Response, error) {
				if oauth {
					if r.Header.Get("Authorization") != "Bearer oauth-test" {
						t.Error("missing bearer")
					}
				} else if r.URL.Query().Get("key") != "key-test" {
					t.Error("missing API key")
				}
				var body geminiRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body.Contents) != 4 || body.Contents[0].Role != "user" || body.Contents[1].Role != "model" || body.Contents[2].Role != "function" || len(body.Tools) != 1 {
					t.Errorf("invalid Gemini request: %+v", body)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"answer"},{"text":"!"},{"functionCall":{"name":"lookup","args":{"key":"value"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`))}, nil
			})}
			req := &ChatRequest{Messages: []Message{{Role: "system", Content: "system"}, {Role: "assistant", ToolCalls: []ToolCall{{Name: "lookup", Arguments: map[string]any{"key": "value"}}}}, {Role: "tool", ToolCallID: "lookup", Content: "result"}, {Role: "user", Content: "question"}}, Tools: []ToolDefinition{{Function: FunctionDef{Name: "lookup", Parameters: map[string]any{"type": "object"}}}}, MaxTokens: 100, Temperature: .5}
			for _, model := range []string{"", "custom-model"} {
				req.Model = model
				got, err := p.Chat(context.Background(), req)
				if err != nil || got.Content != "answer!" || got.Usage.TotalTokens != 5 || len(got.ToolCalls) != 1 || got.ToolCalls[0].Arguments["key"] != "value" {
					t.Fatalf("chat response: %+v %v", got, err)
				}
			}
			if _, err := p.Transcribe(context.Background(), &AudioRequest{}); err == nil {
				t.Fatal("unsupported transcription accepted")
			}
			if _, err := p.Speak(context.Background(), &TTSRequest{}); err == nil {
				t.Fatal("unsupported TTS accepted")
			}
		})
	}
}

func TestGeminiErrors(t *testing.T) {
	p := NewGeminiProvider("test", "custom")
	for _, body := range []string{"broken", "{}"} {
		if _, err := p.parseGeminiResponse([]byte(body)); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
	if got, err := p.parseGeminiResponse([]byte(`{"candidates":[{"content":{"parts":[{"text":"no usage"}]}}]}`)); err != nil || got.Content != "no usage" {
		t.Fatalf("missing usage: %+v %v", got, err)
	}
	for _, mode := range []string{"transport", "read", "status"} {
		p.httpClient = &http.Client{Transport: contractTransport(func(r *http.Request) (*http.Response, error) {
			if mode == "transport" {
				return nil, errors.New("offline")
			}
			status := 200
			var body io.ReadCloser = brokenBody{}
			if mode == "status" {
				status = 403
				body = io.NopCloser(strings.NewReader("denied"))
			}
			return &http.Response{StatusCode: status, Body: body}, nil
		})}
		if _, err := p.Chat(context.Background(), &ChatRequest{}); err == nil {
			t.Fatal("failure hidden")
		}
	}
	if _, err := p.Chat(nil, &ChatRequest{}); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := p.Chat(context.Background(), &ChatRequest{Tools: []ToolDefinition{{Function: FunctionDef{Parameters: map[string]any{"bad": make(chan int)}}}}}); err == nil {
		t.Fatal("unserializable tools accepted")
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	cli := NewGeminiCLIProvider("custom")
	if _, err := cli.Chat(context.Background(), &ChatRequest{}); err == nil {
		t.Fatal("missing CLI credential accepted")
	}
	codex := NewCodexProvider("")
	if codex.DefaultModel() != "gpt-5.3-codex" {
		t.Fatal("default model changed")
	}
	if _, err := codex.Chat(context.Background(), &ChatRequest{}); err == nil {
		t.Fatal("missing credential accepted")
	}
	if _, err := codex.Speak(context.Background(), &TTSRequest{}); err == nil {
		t.Fatal("missing credential accepted")
	}
	if _, err := codex.Transcribe(context.Background(), &AudioRequest{}); err == nil {
		t.Fatal("missing credential accepted")
	}
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex/auth.json"), []byte(`{"access_token":"test-token","expires_at":4102444800}`), 0600); err != nil {
		t.Fatal(err)
	}
	inner, err := NewCodexProvider("custom").resolveInner()
	if err != nil || inner.apiKey != "test-token" || inner.defaultModel != "custom" {
		t.Fatalf("resolved provider: %+v %v", inner, err)
	}
}

func TestLocalWhisperExecution(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "whisper")
	body := `#!/bin/sh
file="$1"
shift
while [ "$#" -gt 0 ]; do
 case "$1" in
 --model) model="$2"; shift 2;;
 --output_dir) out="$2"; shift 2;;
 *) shift;;
 esac
done
[ "$model" = "missing-output" ] && exit 0
[ "$model" = "fail" ] && exit 1
base="${file##*/}"
printf '  recognized speech  \n' > "$out/${base%.*}.txt"
`
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	inner := NewOpenAIProvider("test", "https://example.invalid", "test-model")
	inner.httpClient = &http.Client{Transport: contractTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"choices":[{"message":{"content":"chat"}}]}`
		if strings.HasSuffix(r.URL.Path, "speech") {
			body = "audio"
		}
		if strings.HasSuffix(r.URL.Path, "transcriptions") {
			body = `{"text":"remote"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	p := NewLocalWhisperProvider(config.LocalWhisperConfig{Enabled: true, BinaryPath: script, Model: "local"}, inner)
	if p.DefaultModel() != "test-model" {
		t.Fatal("model delegation failed")
	}
	if got, err := p.Chat(context.Background(), &ChatRequest{}); err != nil || got.Content != "chat" {
		t.Fatalf("chat delegation: %+v %v", got, err)
	}
	if got, err := p.Speak(context.Background(), &TTSRequest{}); err != nil || string(got.AudioData) != "audio" {
		t.Fatalf("speech delegation: %+v %v", got, err)
	}
	for _, model := range []string{"", "override", "fail", "missing-output"} {
		got, err := p.Transcribe(context.Background(), &AudioRequest{FilePath: "recording.wav", Model: model})
		if model == "fail" || model == "missing-output" {
			if err == nil {
				t.Fatal("whisper failure hidden")
			}
		} else if err != nil || got.Text != "recognized speech" {
			t.Fatalf("local transcription: %+v %v", got, err)
		}
	}
	file := filepath.Join(root, "audio.wav")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	p.config.Enabled = false
	if got, err := p.Transcribe(context.Background(), &AudioRequest{FilePath: file}); err != nil || got.Text != "remote" {
		t.Fatalf("transcription fallback: %+v %v", got, err)
	}
}
