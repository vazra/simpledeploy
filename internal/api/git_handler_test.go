package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGitStatus_NilSyncer(t *testing.T) {
	srv, _ := newTestServer(t)
	// gs is nil by default

	req := httptest.NewRequest("GET", "/api/git/status", nil)
	w := httptest.NewRecorder()
	srv.handleGitStatus(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleGitWebhook_NilSyncer(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/git/webhook", nil)
	w := httptest.NewRecorder()
	srv.handleGitWebhook(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
