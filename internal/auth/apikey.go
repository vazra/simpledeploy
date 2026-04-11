package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateAPIKey generates a 32-byte random key, hex-encoded with "sd_" prefix.
// Returns plaintext (shown once) and its SHA-256 hash (stored).
func GenerateAPIKey() (plaintext string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = "sd_" + hex.EncodeToString(b)
	hash = HashAPIKey(plaintext)
	return plaintext, hash, nil
}

// HashAPIKey returns the SHA-256 hex hash of key for deterministic lookup.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// GenerateRandomSecret generates a cryptographically random hex string of n bytes.
func GenerateRandomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
