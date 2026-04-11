package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateAPIKey generates a 32-byte random key, hex-encoded with "sd_" prefix.
// Returns plaintext (shown once) and its HMAC-SHA256 hash (stored).
func GenerateAPIKey(secret string) (plaintext string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = "sd_" + hex.EncodeToString(b)
	hash = HashAPIKey(plaintext, secret)
	return plaintext, hash, nil
}

// HashAPIKey returns the HMAC-SHA256 hex digest of key using secret.
func HashAPIKey(key, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateRandomSecret generates a cryptographically random hex string of n bytes.
func GenerateRandomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
