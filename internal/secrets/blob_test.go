package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
)

// ---------- EncryptBlobWithKey / DecryptBlobWithKey roundtrip ----------

func TestEncryptDecryptBlobWithKey_Roundtrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	plain := []byte("hello, KafClaw secrets!")
	encrypted, err := EncryptBlobWithKey(plain, key)
	if err != nil {
		t.Fatalf("EncryptBlobWithKey: %v", err)
	}

	decrypted, err := DecryptBlobWithKey(encrypted, key)
	if err != nil {
		t.Fatalf("DecryptBlobWithKey: %v", err)
	}

	if string(decrypted) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", decrypted, plain)
	}
}

// ---------- DecryptBlobWithKey backward compat (plaintext JSON) ----------

func TestDecryptBlobWithKey_PlaintextJSON(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	// Plain JSON without version/nonce/ciphertext should be returned as-is.
	plainJSON := []byte(`{"foo":"bar","num":42}`)
	got, err := DecryptBlobWithKey(plainJSON, key)
	if err != nil {
		t.Fatalf("DecryptBlobWithKey plaintext JSON: %v", err)
	}
	if string(got) != string(plainJSON) {
		t.Fatalf("expected plaintext passthrough, got %q", got)
	}
}

// ---------- DecryptBlobWithKey empty input ----------

func TestDecryptBlobWithKey_EmptyInput(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	_, err := DecryptBlobWithKey([]byte{}, key)
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

// ---------- DecryptBlobWithKey wrong key ----------

func TestDecryptBlobWithKey_WrongKey(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	plain := []byte("secret payload")
	encrypted, err := EncryptBlobWithKey(plain, key)
	if err != nil {
		t.Fatalf("EncryptBlobWithKey: %v", err)
	}

	wrongKey := make([]byte, 32)
	if _, err := rand.Read(wrongKey); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	_, err = DecryptBlobWithKey(encrypted, wrongKey)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
}

// ---------- DecodeMasterKey ----------

func TestDecodeMasterKey_Valid(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)

	decoded, err := DecodeMasterKey(encoded)
	if err != nil {
		t.Fatalf("DecodeMasterKey: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(decoded))
	}
	for i := range key {
		if decoded[i] != key[i] {
			t.Fatalf("decoded key mismatch at byte %d", i)
		}
	}
}

func TestDecodeMasterKey_WrongLength(t *testing.T) {
	short := make([]byte, 16)
	if _, err := rand.Read(short); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(short)

	_, err := DecodeMasterKey(encoded)
	if err == nil {
		t.Fatal("expected error for wrong key length, got nil")
	}
}

func TestDecodeMasterKey_InvalidBase64(t *testing.T) {
	_, err := DecodeMasterKey("!!!not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// ---------- DecodeLocalTomb ----------

func TestDecodeLocalTomb_ValidJSON(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)

	tombJSON, _ := json.Marshal(LocalTomb{
		Version:   "v1",
		MasterKey: encoded,
	})

	doc, err := DecodeLocalTomb(tombJSON)
	if err != nil {
		t.Fatalf("DecodeLocalTomb: %v", err)
	}
	if doc.MasterKey != encoded {
		t.Fatalf("master key mismatch: got %q, want %q", doc.MasterKey, encoded)
	}
	if doc.Version != "v1" {
		t.Fatalf("version mismatch: got %q, want %q", doc.Version, "v1")
	}
}

func TestDecodeLocalTomb_RawBase64MasterKey(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)

	doc, err := DecodeLocalTomb([]byte(encoded))
	if err != nil {
		t.Fatalf("DecodeLocalTomb raw base64: %v", err)
	}
	if doc.MasterKey != encoded {
		t.Fatalf("master key mismatch: got %q, want %q", doc.MasterKey, encoded)
	}
	if doc.Version != "v1" {
		t.Fatalf("expected version v1, got %q", doc.Version)
	}
}

func TestDecodeLocalTomb_EmptyInput(t *testing.T) {
	_, err := DecodeLocalTomb([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestDecodeLocalTomb_InvalidFormat(t *testing.T) {
	_, err := DecodeLocalTomb([]byte("this-is-not-valid-anything"))
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
}

// ---------- LoadEnvSecretsFromTombDoc / SealEnvSecretsIntoTombDoc roundtrip ----------

func TestSealAndLoadEnvSecrets_Roundtrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)

	doc := &LocalTomb{
		Version:   "v1",
		MasterKey: encoded,
	}

	secrets := map[string]string{
		"API_KEY":   "sk-12345",
		"DB_PASS":   "hunter2",
		"EMPTY_VAL": "",
	}

	if err := SealEnvSecretsIntoTombDoc(doc, secrets); err != nil {
		t.Fatalf("SealEnvSecretsIntoTombDoc: %v", err)
	}

	if doc.EnvNonce == "" || doc.EnvCiphertext == "" {
		t.Fatal("expected EnvNonce and EnvCiphertext to be populated after sealing")
	}

	loaded, err := LoadEnvSecretsFromTombDoc(doc)
	if err != nil {
		t.Fatalf("LoadEnvSecretsFromTombDoc: %v", err)
	}

	for k, want := range secrets {
		got, ok := loaded[k]
		if !ok {
			t.Fatalf("missing key %q in loaded secrets", k)
		}
		if got != want {
			t.Fatalf("secret %q mismatch: got %q, want %q", k, got, want)
		}
	}
	if len(loaded) != len(secrets) {
		t.Fatalf("loaded secret count mismatch: got %d, want %d", len(loaded), len(secrets))
	}
}

// ---------- SealEnvSecretsIntoTombDoc with nil doc ----------

func TestSealEnvSecretsIntoTombDoc_NilDoc(t *testing.T) {
	err := SealEnvSecretsIntoTombDoc(nil, map[string]string{"a": "b"})
	if err == nil {
		t.Fatal("expected error for nil doc, got nil")
	}
}

// ---------- LoadEnvSecretsFromTombDoc with empty nonce/ciphertext ----------

func TestLoadEnvSecretsFromTombDoc_EmptyNonceCiphertext(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)

	doc := &LocalTomb{
		Version:   "v1",
		MasterKey: encoded,
	}

	got, err := LoadEnvSecretsFromTombDoc(doc)
	if err != nil {
		t.Fatalf("LoadEnvSecretsFromTombDoc: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}
