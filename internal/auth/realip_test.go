package auth

import (
	"net/http/httptest"
	"testing"
)

func TestRealIP_NoProxies(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	if got := RealIP(r, nil); got != "1.2.3.4" {
		t.Fatalf("got %q, want 1.2.3.4", got)
	}
}

func TestRealIP_TrustedExactIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	if got := RealIP(r, []string{"10.0.0.1"}); got != "203.0.113.5" {
		t.Fatalf("got %q, want 203.0.113.5", got)
	}
}

func TestRealIP_TrustedCIDR(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.50:1234"
	r.Header.Set("X-Forwarded-For", "8.8.8.8")
	if got := RealIP(r, []string{"10.0.0.0/8"}); got != "8.8.8.8" {
		t.Fatalf("CIDR not honored: got %q, want 8.8.8.8", got)
	}
}

func TestRealIP_UntrustedDirectIgnoresXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "9.9.9.9:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := RealIP(r, []string{"10.0.0.0/8"}); got != "9.9.9.9" {
		t.Fatalf("got %q, want 9.9.9.9 (XFF must be ignored from untrusted hop)", got)
	}
}

func TestRealIP_MalformedTrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.50:1234"
	r.Header.Set("X-Forwarded-For", "8.8.8.8")
	// Malformed entries are silently dropped from the trust list.
	if got := RealIP(r, []string{"not-an-ip", "also/garbage"}); got != "10.0.0.50" {
		t.Fatalf("got %q, want 10.0.0.50 (no valid trusted entries)", got)
	}
}
