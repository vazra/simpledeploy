package auth

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	plain, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey error: %v", err)
	}
	if !strings.HasPrefix(plain, "sd_") {
		t.Errorf("expected plaintext to start with sd_, got %s", plain)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	// Hash is deterministic: rehashing the plaintext should match
	if HashAPIKey(plain) != hash {
		t.Error("HashAPIKey(plain) should equal the returned hash")
	}
}

func TestHashAPIKeyDeterministic(t *testing.T) {
	key := "sd_somekey"
	h1 := HashAPIKey(key)
	h2 := HashAPIKey(key)
	if h1 != h2 {
		t.Error("HashAPIKey should be deterministic")
	}
	if h1 == "" {
		t.Error("expected non-empty hash")
	}
}

func TestGenerateAPIKeyUnique(t *testing.T) {
	plain1, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey error: %v", err)
	}
	plain2, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey error: %v", err)
	}
	if plain1 == plain2 {
		t.Error("two GenerateAPIKey calls should produce different keys")
	}
}
