package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type lifecycleEvent struct {
	Timestamp string         `json:"timestamp"`
	Command   string         `json:"command"`
	Action    string         `json:"action"`
	Status    string         `json:"status"`
	Message   string         `json:"message,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

func emitLifecycleEvent(command, action, status, message string, fields map[string]any) error {
	home, err := resolveLifecycleHome()
	if err != nil {
		return err
	}
	logDir := filepath.Join(home, ".kafclaw")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(logDir, "lifecycle-events.jsonl")

	ev := lifecycleEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Command:   strings.TrimSpace(command),
		Action:    strings.TrimSpace(action),
		Status:    strings.TrimSpace(status),
		Message:   strings.TrimSpace(message),
		Fields:    fields,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func resolveLifecycleHome() (string, error) {
	if h := strings.TrimSpace(os.Getenv("KAFCLAW_HOME")); h != "" {
		return h, nil
	}
	if h := strings.TrimSpace(os.Getenv("MIKROBOT_HOME")); h != "" {
		return h, nil
	}
	return os.UserHomeDir()
}
