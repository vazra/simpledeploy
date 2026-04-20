package gitsync

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
)

// newWebhookHandler returns a GitHub-compatible webhook handler.
// It verifies X-Hub-Signature-256: sha256=<hex> and, on success, enqueues a SyncNow.
func newWebhookHandler(g *Syncer) http.Handler {
	secret := []byte(g.cfg.WebhookSecret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyHMAC(secret, body, sig) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Non-blocking: enqueue sync.
		go func() {
			_ = g.SyncNow(r.Context())
		}()

		w.WriteHeader(http.StatusAccepted)
	})
}

// verifyHMAC checks the X-Hub-Signature-256 value.
// sig has the form "sha256=<hex>".
func verifyHMAC(secret, body []byte, sig string) bool {
	if !strings.HasPrefix(sig, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(sig, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(got, expected)
}
