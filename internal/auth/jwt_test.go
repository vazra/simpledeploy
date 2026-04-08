package auth

import (
	"testing"
	"time"
)

func TestJWTGenerateAndValidate(t *testing.T) {
	mgr := NewJWTManager("supersecret", time.Hour)
	token, err := mgr.Generate(42, "alice", "admin")
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := mgr.Validate(token)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("expected UserID 42, got %d", claims.UserID)
	}
	if claims.Username != "alice" {
		t.Errorf("expected username alice, got %s", claims.Username)
	}
	if claims.Role != "admin" {
		t.Errorf("expected role admin, got %s", claims.Role)
	}
}

func TestJWTExpired(t *testing.T) {
	mgr := NewJWTManager("supersecret", time.Millisecond)
	token, err := mgr.Generate(1, "bob", "user")
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	_, err = mgr.Validate(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestJWTInvalidSignature(t *testing.T) {
	mgr1 := NewJWTManager("secret1", time.Hour)
	mgr2 := NewJWTManager("secret2", time.Hour)

	token, err := mgr1.Generate(1, "carol", "user")
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	_, err = mgr2.Validate(token)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}
