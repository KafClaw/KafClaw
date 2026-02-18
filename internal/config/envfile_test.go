package config

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFileParsesAndRespectsExistingValues(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "env")
	content := `
# comment
export FOO=bar
QUOTED="hello world"
SINGLE='x y'
INVALID_LINE
`
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	origFoo := os.Getenv("FOO")
	origQuoted := os.Getenv("QUOTED")
	defer os.Setenv("FOO", origFoo)
	defer os.Setenv("QUOTED", origQuoted)

	_ = os.Setenv("FOO", "existing")
	_ = os.Unsetenv("QUOTED")
	_ = os.Unsetenv("SINGLE")

	if err := loadEnvFile(envPath); err != nil {
		t.Fatalf("load env file: %v", err)
	}

	if got := os.Getenv("FOO"); got != "existing" {
		t.Fatalf("expected existing FOO preserved, got %q", got)
	}
	if got := os.Getenv("QUOTED"); got != "hello world" {
		t.Fatalf("expected QUOTED loaded, got %q", got)
	}
	if got := os.Getenv("SINGLE"); got != "x y" {
		t.Fatalf("expected SINGLE loaded, got %q", got)
	}
}

func TestLoadEnvFileCandidatesFromExplicitPath(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "kafclaw.env")
	if err := os.WriteFile(envPath, []byte("EXPLICIT_KEY=42\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	origExplicit := os.Getenv("KAFCLAW_ENV_FILE")
	origKey := os.Getenv("EXPLICIT_KEY")
	defer os.Setenv("KAFCLAW_ENV_FILE", origExplicit)
	defer os.Setenv("EXPLICIT_KEY", origKey)
	_ = os.Setenv("KAFCLAW_ENV_FILE", envPath)
	_ = os.Unsetenv("EXPLICIT_KEY")

	LoadEnvFileCandidates()

	if got := os.Getenv("EXPLICIT_KEY"); got != "42" {
		t.Fatalf("expected EXPLICIT_KEY loaded from explicit env file, got %q", got)
	}
}

func TestLoadEnvFileCandidatesLoadsSecretsFromTomb(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	tombPath := filepath.Join(home, ".config", "kafclaw", localTombFileName)
	if err := os.MkdirAll(filepath.Dir(tombPath), 0o700); err != nil {
		t.Fatalf("mkdir tomb dir: %v", err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	plain, err := json.Marshal(map[string]string{
		"OPENAI_API_KEY": "from_tomb",
	})
	if err != nil {
		t.Fatalf("marshal plain: %v", err)
	}
	nonce := []byte("0123456789ab")
	ciphertext := gcm.Seal(nil, nonce, plain, []byte(localTombEnvAAD))
	doc := localTombDoc{
		Version:       "v1",
		MasterKey:     base64.RawStdEncoding.EncodeToString(key),
		EnvNonce:      base64.RawStdEncoding.EncodeToString(nonce),
		EnvCiphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	docData, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal tomb doc: %v", err)
	}
	if err := os.WriteFile(tombPath, append(docData, '\n'), 0o600); err != nil {
		t.Fatalf("write tomb file: %v", err)
	}

	origHome := os.Getenv("HOME")
	origKey := os.Getenv("OPENAI_API_KEY")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("OPENAI_API_KEY", origKey)
	_ = os.Setenv("HOME", home)
	_ = os.Unsetenv("OPENAI_API_KEY")

	LoadEnvFileCandidates()

	if got := os.Getenv("OPENAI_API_KEY"); got != "from_tomb" {
		t.Fatalf("expected OPENAI_API_KEY loaded from tomb, got %q", got)
	}

	_ = os.Setenv("OPENAI_API_KEY", "already_set")
	LoadEnvFileCandidates()
	if got := os.Getenv("OPENAI_API_KEY"); got != "already_set" {
		t.Fatalf("expected existing env key preserved, got %q", got)
	}
}

func TestResolveLocalTombPathWithExplicitAndTilde(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	origKafclawHome := os.Getenv("KAFCLAW_HOME")
	origTomb := os.Getenv("KAFCLAW_OAUTH_TOMB_FILE")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKafclawHome)
	defer os.Setenv("KAFCLAW_OAUTH_TOMB_FILE", origTomb)

	home := filepath.Join(tmp, "home")
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("KAFCLAW_HOME", filepath.Join(tmp, "cfg-home"))
	_ = os.Setenv("KAFCLAW_OAUTH_TOMB_FILE", "~/secrets/custom.rr")

	got, err := resolveLocalTombPath()
	if err != nil {
		t.Fatalf("resolveLocalTombPath error: %v", err)
	}
	want := filepath.Join(tmp, "cfg-home", "secrets", "custom.rr")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveLocalTombPathDefaultHome(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	origKafclawHome := os.Getenv("KAFCLAW_HOME")
	origTomb := os.Getenv("KAFCLAW_OAUTH_TOMB_FILE")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKafclawHome)
	defer os.Setenv("KAFCLAW_OAUTH_TOMB_FILE", origTomb)

	home := filepath.Join(tmp, "home")
	_ = os.Setenv("HOME", home)
	_ = os.Unsetenv("KAFCLAW_HOME")
	_ = os.Unsetenv("KAFCLAW_OAUTH_TOMB_FILE")

	got, err := resolveLocalTombPath()
	if err != nil {
		t.Fatalf("resolveLocalTombPath error: %v", err)
	}
	want := filepath.Join(home, ".config", "kafclaw", localTombFileName)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveEnvHomeDirAndExpandTildePath(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	origKafclawHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKafclawHome)

	home := filepath.Join(tmp, "home")
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("KAFCLAW_HOME", "~/custom")

	got, err := resolveEnvHomeDir()
	if err != nil {
		t.Fatalf("resolveEnvHomeDir error: %v", err)
	}
	want := filepath.Join(home, "custom")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	if out := expandTildePath("~", home); out != home {
		t.Fatalf("expected %q for bare tilde, got %q", home, out)
	}
	if out := expandTildePath("~/a/b", home); out != filepath.Join(home, "a", "b") {
		t.Fatalf("unexpected expanded subpath: %q", out)
	}
	if out := expandTildePath("/abs/path", home); out != "/abs/path" {
		t.Fatalf("expected absolute path unchanged, got %q", out)
	}
}

func TestDecodeLocalTombDocCompatibilityAndValidation(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	encoded := base64.RawStdEncoding.EncodeToString(key)

	doc, err := decodeLocalTombDoc([]byte(encoded))
	if err != nil {
		t.Fatalf("decodeLocalTombDoc raw key error: %v", err)
	}
	if doc.Version != "v1" || doc.MasterKey != encoded {
		t.Fatalf("unexpected compatibility decode result: %+v", doc)
	}

	_, err = decodeLocalTombDoc([]byte("   "))
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected os.ErrInvalid for blank doc, got %v", err)
	}
}

func TestDecodeMasterKeyRejectsInvalidLength(t *testing.T) {
	short := base64.RawStdEncoding.EncodeToString([]byte("short-key"))
	_, err := decodeMasterKey(short)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected os.ErrInvalid for short key, got %v", err)
	}
}
