package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltSize = 16
	// pbkdf2IterModern is the iteration count used for new encryptions
	// (OWASP 2023 PBKDF2-HMAC-SHA256 baseline). pbkdf2IterLegacy is the
	// pre-bump value; Decrypt tries both for backwards compatibility.
	pbkdf2IterModern = 600_000
	pbkdf2IterLegacy = 100_000
)

// Encrypt encrypts plaintext with AES-256-GCM using a random salt + PBKDF2 key.
// Returns base64-encoded salt+nonce+ciphertext.
func Encrypt(plaintext, key string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	block, err := aes.NewCipher(deriveKeyN(key, salt, pbkdf2IterModern))
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	out := append(salt, ciphertext...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt decrypts base64-encoded salt+nonce+ciphertext with AES-256-GCM.
// Tries iterations in modern -> legacy order, then falls back to the
// fixed-salt legacy format for backwards compatibility.
func Decrypt(encoded, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	if len(data) > saltSize {
		salt, ct := data[:saltSize], data[saltSize:]
		if pt, err := decryptWithSaltN(salt, ct, key, pbkdf2IterModern); err == nil {
			return pt, nil
		}
		if pt, err := decryptWithSaltN(salt, ct, key, pbkdf2IterLegacy); err == nil {
			return pt, nil
		}
	}
	// Fallback: legacy fixed salt format (nonce + ciphertext)
	return decryptWithSaltN([]byte("simpledeploy-v1"), data, key, pbkdf2IterLegacy)
}

func decryptWithSaltN(salt, data []byte, key string, iter int) (string, error) {
	block, err := aes.NewCipher(deriveKeyN(key, salt, iter))
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

func deriveKeyN(key string, salt []byte, iter int) []byte {
	return pbkdf2.Key([]byte(key), salt, iter, 32, sha256.New)
}
