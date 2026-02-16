package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kamir/gomikrobot/internal/config"
)

// LocalWhisperProvider implements transcription using a local Whisper binary.
type LocalWhisperProvider struct {
	config config.LocalWhisperConfig
	openai *OpenAIProvider // Fallback or for non-transcription tasks
}

// NewLocalWhisperProvider creates a new local Whisper provider.
func NewLocalWhisperProvider(cfg config.LocalWhisperConfig, openai *OpenAIProvider) *LocalWhisperProvider {
	return &LocalWhisperProvider{
		config: cfg,
		openai: openai,
	}
}

func (p *LocalWhisperProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return p.openai.Chat(ctx, req)
}

func (p *LocalWhisperProvider) Speak(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	return p.openai.Speak(ctx, req)
}

func (p *LocalWhisperProvider) DefaultModel() string {
	return p.openai.DefaultModel()
}

// Transcribe converts audio to text using a local Command Line Whisper.
func (p *LocalWhisperProvider) Transcribe(ctx context.Context, req *AudioRequest) (*AudioResponse, error) {
	if !p.config.Enabled {
		return p.openai.Transcribe(ctx, req)
	}

	model := req.Model
	if model == "" {
		model = p.config.Model
	}

	// Create a temporary directory for output
	tmpDir, err := os.MkdirTemp("", "whisper-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Command: whisper <file> --model <model> --output_dir <tmpDir> --output_format txt --language de
	args := []string{
		req.FilePath,
		"--model", model,
		"--output_dir", tmpDir,
		"--output_format", "txt",
		"--language", "de", // Enforce German
		"--verbose", "False",
	}

	cmd := exec.CommandContext(ctx, p.config.BinaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("whisper command failed: %w (output: %s)", err, string(output))
	}

	// Read the output txt file
	// Whisper usually outputs <filename>.txt
	base := filepath.Base(req.FilePath)
	// Remove extension
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	txtPath := filepath.Join(tmpDir, name+".txt")

	txtData, err := os.ReadFile(txtPath)
	if err != nil {
		return nil, fmt.Errorf("read transcription output: %w", err)
	}

	return &AudioResponse{
		Text: strings.TrimSpace(string(txtData)),
	}, nil
}
