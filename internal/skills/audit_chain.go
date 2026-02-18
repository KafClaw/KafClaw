package skills

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func appendChainedAuditLine(path string, payload any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	entry, err := normalizeAuditPayload(payload)
	if err != nil {
		return err
	}
	prevHash, err := readLastAuditHash(path)
	if err != nil {
		return err
	}
	if prevHash != "" {
		entry["prevHash"] = prevHash
	}
	hash := computeAuditHash(entry)
	entry["hash"] = hash
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func normalizeAuditPayload(payload any) (map[string]any, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	delete(out, "hash")
	delete(out, "prevHash")
	return out, nil
}

func readLastAuditHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	var last string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			last = line
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if last == "" {
		return "", nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(last), &obj); err != nil {
		return "", fmt.Errorf("invalid existing audit line: %w", err)
	}
	return strings.TrimSpace(toString(obj["hash"])), nil
}

func computeAuditHash(entry map[string]any) string {
	canonical := map[string]any{}
	for k, v := range entry {
		if k == "hash" {
			continue
		}
		canonical[k] = v
	}
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func appendSecurityAuditEvent(eventType string, payload map[string]any) error {
	state, err := EnsureStateDirs()
	if err != nil {
		return err
	}
	event := map[string]any{
		"time":      time.Now().UTC(),
		"eventType": strings.TrimSpace(eventType),
	}
	for k, v := range payload {
		event[k] = v
	}
	return appendChainedAuditLine(filepath.Join(state.AuditDir, "security-events.jsonl"), event)
}
