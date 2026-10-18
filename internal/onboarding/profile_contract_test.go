package onboarding

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

type failingInput struct{}

func (failingInput) Read([]byte) (int, error) { return 0, errors.New("input failed") }

func TestInteractiveProfileSelections(t *testing.T) {
	for _, tc := range []struct {
		choice string
		want   RuntimeMode
	}{{"1", ModeLocal}, {"2", ModeLocalKafka}, {"3", ModeRemote}} {
		got, err := resolveMode(bufio.NewReader(strings.NewReader(tc.choice+"\n")), io.Discard, WizardParams{})
		if err != nil || got != tc.want {
			t.Fatalf("mode %s: %s %v", tc.choice, got, err)
		}
	}
	presets := []LLMPreset{LLMPresetClaude, LLMPresetCLIToken, LLMPresetGemini, LLMPresetGeminiCLI, LLMPresetCodex, LLMPresetXAI, LLMPresetOpenRouter, LLMPresetDeepSeek, LLMPresetGroq, LLMPresetScalyticsCopilot, LLMPresetOpenAICompatible, LLMPresetSkip}
	for i, want := range presets {
		got, err := resolveLLMPreset(bufio.NewReader(strings.NewReader(fmt.Sprint(i+1)+"\n")), io.Discard, WizardParams{})
		if err != nil || got != want {
			t.Fatalf("preset %d: %s %v", i+1, got, err)
		}
	}
	for _, resolve := range []func(*bufio.Reader, io.Writer, WizardParams) (string, error){func(r *bufio.Reader, w io.Writer, p WizardParams) (string, error) {
		v, e := resolveMode(r, w, p)
		return string(v), e
	}, func(r *bufio.Reader, w io.Writer, p WizardParams) (string, error) {
		v, e := resolveLLMPreset(r, w, p)
		return string(v), e
	}} {
		for _, r := range []io.Reader{strings.NewReader("invalid\n"), failingInput{}} {
			if _, err := resolve(bufio.NewReader(r), io.Discard, WizardParams{}); err == nil {
				t.Fatal("invalid input accepted")
			}
		}
	}
	for _, preset := range presets {
		cfg := config.DefaultConfig()
		var out bytes.Buffer
		if err := applyLLM(cfg, preset, bufio.NewReader(strings.NewReader("fixture\nfixture\nfixture\n")), &out, WizardParams{}); err != nil {
			t.Fatalf("interactive %s: %v", preset, err)
		}
		if preset != LLMPresetSkip && cfg.Model.Name == "" {
			t.Fatal("empty model")
		}
	}
}

func TestProfileInputErrors(t *testing.T) {
	for _, preset := range []LLMPreset{LLMPresetCLIToken, LLMPresetClaude, LLMPresetScalyticsCopilot, LLMPresetOpenAICompatible} {
		for _, prefix := range []string{"", "fixture\n", "fixture\nfixture\n"} {
			r := io.MultiReader(strings.NewReader(prefix), failingInput{})
			err := applyLLM(config.DefaultConfig(), preset, bufio.NewReader(r), io.Discard, WizardParams{})
			if len(strings.Split(prefix, "\n")) <= 2 && err == nil {
				t.Fatalf("input error hidden %s prefix=%q", preset, prefix)
			}
		}
	}
	cfg := config.DefaultConfig()
	if err := applyLLM(cfg, LLMPreset("invalid"), bufio.NewReader(strings.NewReader("")), io.Discard, WizardParams{}); err == nil {
		t.Fatal("unknown preset accepted")
	}
	if err := applyMode(cfg, RuntimeMode("invalid"), WizardParams{}); err == nil {
		t.Fatal("unknown mode accepted")
	}
	if err := RunProfileWizard(cfg, failingInput{}, io.Discard, WizardParams{}); err == nil {
		t.Fatal("mode input error hidden")
	}
	if err := RunProfileWizard(cfg, failingInput{}, io.Discard, WizardParams{Mode: "local"}); err == nil {
		t.Fatal("provider input error hidden")
	}
	if err := RunProfileWizard(cfg, failingInput{}, io.Discard, WizardParams{Mode: "local-kafka", KafkaSecurity: "invalid"}); err == nil {
		t.Fatal("invalid Kafka security accepted")
	}
	if _, err := ConfirmApply(bufio.NewReader(failingInput{}), io.Discard); err == nil {
		t.Fatal("confirmation input error hidden")
	}
}
