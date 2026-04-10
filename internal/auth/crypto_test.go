package auth

import "testing"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := "my-secret-key-for-testing"
	plaintext := "hunter2"

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if encrypted == plaintext {
		t.Fatal("encrypted should differ from plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	encrypted, err := Encrypt("secret", "key1")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = Decrypt(encrypted, "key2")
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestEncryptEmptyString(t *testing.T) {
	encrypted, err := Encrypt("", "key")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := Decrypt(encrypted, "key")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != "" {
		t.Errorf("got %q, want empty", decrypted)
	}
}
