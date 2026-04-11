package auth

import (
	"strings"
	"testing"
)

const testSecret = "test-hmac-secret"

func TestGenerateAPIKey(t *testing.T) {
	plain, hash, err := GenerateAPIKey(testSecret)
	if err != nil {
		t.Fatalf("GenerateAPIKey error: %v", err)
	}
	if !strings.HasPrefix(plain, "sd_") {
		t.Errorf("expected plaintext to start with sd_, got %s", plain)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if HashAPIKey(plain, testSecret) != hash {
		t.Error("HashAPIKey(plain, secret) should equal the returned hash")
	}
}

func TestHashAPIKeyDeterministic(t *testing.T) {
	key := "sd_somekey"
	h1 := HashAPIKey(key, testSecret)
	h2 := HashAPIKey(key, testSecret)
	if h1 != h2 {
		t.Error("HashAPIKey should be deterministic")
	}
	if h1 == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHashAPIKeyDifferentSecrets(t *testing.T) {
	key := "sd_somekey"
	h1 := HashAPIKey(key, "secret-a")
	h2 := HashAPIKey(key, "secret-b")
	if h1 == h2 {
		t.Error("different secrets should produce different hashes")
	}
}

func TestGenerateAPIKeyUnique(t *testing.T) {
	plain1, _, err := GenerateAPIKey(testSecret)
	if err != nil {
		t.Fatalf("GenerateAPIKey error: %v", err)
	}
	plain2, _, err := GenerateAPIKey(testSecret)
	if err != nil {
		t.Fatalf("GenerateAPIKey error: %v", err)
	}
	if plain1 == plain2 {
		t.Error("two GenerateAPIKey calls should produce different keys")
	}
}
