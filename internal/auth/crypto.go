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

const saltSize = 16

// Encrypt encrypts plaintext with AES-256-GCM using a random salt + PBKDF2 key.
// Returns base64-encoded salt+nonce+ciphertext.
func Encrypt(plaintext, key string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	block, err := aes.NewCipher(deriveKey(key, salt))
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
// Falls back to legacy fixed-salt format for backwards compatibility.
func Decrypt(encoded, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	// Try new format: salt(16) + nonce + ciphertext
	if len(data) > saltSize {
		if pt, err := decryptWithSalt(data[:saltSize], data[saltSize:], key); err == nil {
			return pt, nil
		}
	}
	// Fallback: legacy fixed salt format (nonce + ciphertext)
	return decryptWithSalt([]byte("simpledeploy-v1"), data, key)
}

func decryptWithSalt(salt, data []byte, key string) (string, error) {
	block, err := aes.NewCipher(deriveKey(key, salt))
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

func deriveKey(key string, salt []byte) []byte {
	return pbkdf2.Key([]byte(key), salt, 100_000, 32, sha256.New)
}
