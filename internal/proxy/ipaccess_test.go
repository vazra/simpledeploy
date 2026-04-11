package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPAccessRegistryNoRules(t *testing.T) {
	reg := newIPAccessRegistry()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	if !reg.Allowed("example.com", req) {
		t.Error("no rules for domain should allow all traffic")
	}
}

func TestIPAccessRegistryExactIP(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{"10.0.0.1", "192.168.1.5"})

	allowed := httptest.NewRequest("GET", "/", nil)
	allowed.RemoteAddr = "192.168.1.5:9999"
	if !reg.Allowed("example.com", allowed) {
		t.Error("exact IP match should be allowed")
	}

	blocked := httptest.NewRequest("GET", "/", nil)
	blocked.RemoteAddr = "5.5.5.5:9999"
	if reg.Allowed("example.com", blocked) {
		t.Error("non-matching IP should be blocked")
	}
}

func TestIPAccessRegistryCIDR(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{"10.0.0.0/8"})

	allowed := httptest.NewRequest("GET", "/", nil)
	allowed.RemoteAddr = "10.50.100.200:1234"
	if !reg.Allowed("example.com", allowed) {
		t.Error("IP in CIDR range should be allowed")
	}

	blocked := httptest.NewRequest("GET", "/", nil)
	blocked.RemoteAddr = "11.0.0.1:1234"
	if reg.Allowed("example.com", blocked) {
		t.Error("IP outside CIDR should be blocked")
	}
}

func TestIPAccessRegistryMixedRules(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{"10.0.0.0/8", "203.0.113.5"})

	tests := []struct {
		addr string
		want bool
	}{
		{"10.1.2.3:80", true},
		{"203.0.113.5:80", true},
		{"203.0.113.6:80", false},
		{"192.168.1.1:80", false},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = tt.addr
		got := reg.Allowed("example.com", req)
		if got != tt.want {
			t.Errorf("Allowed(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestIPAccessRegistryEmptyList(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	if !reg.Allowed("example.com", req) {
		t.Error("empty allowlist should allow all traffic (treated as disabled)")
	}
}

func TestIPAccessHandlerBlocks(t *testing.T) {
	orig := IPAccessRules
	defer func() { IPAccessRules = orig }()

	IPAccessRules = newIPAccessRegistry()
	IPAccessRules.Set("secure.com", []string{"10.0.0.1"})

	h := &IPAccessHandler{}

	// Allowed request
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "secure.com"
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	if err := h.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	if w.Code == http.StatusNotFound {
		t.Error("allowed IP should not get 404")
	}

	// Blocked request
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Host = "secure.com"
	req2.RemoteAddr = "5.5.5.5:1234"
	w2 := httptest.NewRecorder()
	if err := h.ServeHTTP(w2, req2, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	if w2.Code != http.StatusNotFound {
		t.Errorf("blocked IP: got status %d, want 404", w2.Code)
	}
}

func TestIPAccessHandlerNoRules(t *testing.T) {
	orig := IPAccessRules
	defer func() { IPAccessRules = orig }()

	IPAccessRules = newIPAccessRegistry()

	h := &IPAccessHandler{}
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "open.com"
	req.RemoteAddr = "1.2.3.4:5678"
	w := httptest.NewRecorder()
	if err := h.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	if w.Code == http.StatusNotFound {
		t.Error("no rules should allow all traffic")
	}
}

func TestIPAccessHandlerModuleInfo(t *testing.T) {
	h := IPAccessHandler{}
	info := h.CaddyModule()
	if info.ID != "http.handlers.simpledeploy_ipaccess" {
		t.Errorf("module ID: got %q, want %q", info.ID, "http.handlers.simpledeploy_ipaccess")
	}
	if info.New == nil {
		t.Error("New is nil")
	}
}
