package api

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/fsutil"
	"github.com/vazra/simpledeploy/internal/store"
)

var validDomain = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.*-]*$`)

// proxyReloader forces a proxy reload. Custom cert files are rewritten at the
// same path, so the generated proxy config does not change and a plain route
// refresh would be skipped.
type proxyReloader interface {
	ForceReload() error
}

// SetProxyReloader sets the proxy used to apply cert changes.
func (s *Server) SetProxyReloader(p proxyReloader) { s.proxyReloader = p }

// reloadProxyForCert forces a proxy reload so cert changes apply immediately.
// Returns false (and writes an error response) when the reload failed.
func (s *Server) reloadProxyForCert(w http.ResponseWriter, action string) bool {
	if s.proxyReloader == nil {
		return true
	}
	if err := s.proxyReloader.ForceReload(); err != nil {
		// Log the detail; return a fixed message (no internal error text).
		log.Printf("[api] cert %s but proxy reload failed: %v", action, err)
		http.Error(w, "cert "+action+" but proxy reload failed", http.StatusInternalServerError)
		return false
	}
	return true
}

// appEndpointDomain returns the app's endpoint domain equal to domain
// (case-insensitive, trailing dot ignored), spelled as in the compose file so
// the cert file name matches what the proxy loads. "" means domain is not one
// of the app's endpoints.
func appEndpointDomain(app *store.App, domain string) (string, error) {
	cfg, err := compose.ParseFile(app.ComposePath, app.Slug)
	if err != nil {
		return "", err
	}
	key := endpointDomainKey(domain)
	match := ""
	for _, ep := range cfg.Endpoints {
		if ep.Domain == domain {
			return ep.Domain, nil
		}
		if match == "" && endpointDomainKey(ep.Domain) == key && validDomain.MatchString(ep.Domain) {
			match = ep.Domain
		}
	}
	return match, nil
}

// resolveCertDomain maps the requested domain to the app's matching endpoint
// domain. Custom certs may only be managed for the app's own endpoints.
// Writes an error response and returns false otherwise.
func resolveCertDomain(w http.ResponseWriter, app *store.App, domain string) (string, bool) {
	epDomain, err := appEndpointDomain(app, domain)
	if err != nil {
		log.Printf("[api] cert %s/%s: read endpoints: %v", app.Slug, domain, err)
		http.Error(w, "could not read this app's endpoints", http.StatusInternalServerError)
		return "", false
	}
	if epDomain == "" {
		http.Error(w, fmt.Sprintf("domain %s is not one of this app's endpoints; add it as an endpoint first", domain), http.StatusBadRequest)
		return "", false
	}
	return epDomain, true
}

type certRequest struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func (s *Server) handleUploadCert(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	domain := r.PathValue("domain")

	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	if domain == "" || !validDomain.MatchString(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	domain, ok := resolveCertDomain(w, app, domain)
	if !ok {
		return
	}

	var req certRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Cert == "" || req.Key == "" {
		http.Error(w, "cert and key are required", http.StatusBadRequest)
		return
	}

	if block, _ := pem.Decode([]byte(req.Cert)); block == nil || block.Type != "CERTIFICATE" {
		http.Error(w, "cert is not valid PEM", http.StatusBadRequest)
		return
	}
	if block, _ := pem.Decode([]byte(req.Key)); block == nil || !strings.Contains(block.Type, "KEY") {
		http.Error(w, "key is not valid PEM", http.StatusBadRequest)
		return
	}
	if msg := checkCertPair([]byte(req.Cert), []byte(req.Key), domain); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	appDir := filepath.Dir(app.ComposePath)
	certDir := filepath.Join(appDir, "certs")
	if err := fsutil.EnsureNoSymlinks(appDir, certDir); err != nil {
		httpError(w, fmt.Errorf("certs dir: %w", err), http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(certDir, 0755); err != nil {
		httpError(w, fmt.Errorf("create certs dir: %w", err), http.StatusInternalServerError)
		return
	}

	certFile := certFileState{path: filepath.Join(certDir, domain+".crt"), perm: 0o644}
	keyFile := certFileState{path: filepath.Join(certDir, domain+".key"), perm: 0o600}
	certFile.save()
	keyFile.save()
	restore := func() {
		certFile.restore()
		keyFile.restore()
	}

	if err := fsutil.WriteFileAtomic(certFile.path, []byte(req.Cert), certFile.perm); err != nil {
		restore()
		httpError(w, fmt.Errorf("write cert: %w", err), http.StatusInternalServerError)
		return
	}
	if err := fsutil.WriteFileAtomic(keyFile.path, []byte(req.Key), keyFile.perm); err != nil {
		restore()
		httpError(w, fmt.Errorf("write key: %w", err), http.StatusInternalServerError)
		return
	}

	// On a failed reload the proxy keeps serving the previous files, so
	// put them back to keep disk and proxy in step.
	if s.proxyReloader != nil {
		if err := s.proxyReloader.ForceReload(); err != nil {
			restore()
			log.Printf("[api] cert upload %s/%s: proxy reload failed: %v", app.Slug, domain, err)
			http.Error(w, "proxy reload failed, so the new cert was not applied; the previous cert files were kept", http.StatusInternalServerError)
			return
		}
	}

	// Audit: record only domain; never log cert/key bodies.
	s.recordAudit(r, app, "endpoint", "cert_uploaded", nil, map[string]any{"domain": domain})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// checkCertPair verifies that key belongs to the first certificate in
// certPEM and that the certificate covers domain. Returns a message for the
// user, or "" when the pair is usable.
func checkCertPair(certPEM, keyPEM []byte, domain string) string {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return "cert and key do not match or cannot be read; the key must belong to the first certificate in the file"
	}
	leaf := pair.Leaf
	if leaf == nil {
		if leaf, err = x509.ParseCertificate(pair.Certificate[0]); err != nil {
			return "cert cannot be read"
		}
	}
	if err := leaf.VerifyHostname(domain); err != nil {
		return fmt.Sprintf("this certificate is not valid for %s; upload a certificate whose names include it", domain)
	}
	return ""
}

// certFileState remembers a cert or key file's previous content so a failed
// upload can put it back (or remove the new file when there was none).
type certFileState struct {
	path string
	perm os.FileMode
	prev []byte
	had  bool
}

func (c *certFileState) save() {
	data, err := fsutil.ReadRegularFile(c.path)
	if err == nil {
		c.prev, c.had = data, true
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("[api] cert: cannot keep previous %s: %v", c.path, err)
	}
}

func (c *certFileState) restore() {
	var err error
	if c.had {
		err = fsutil.WriteFileAtomic(c.path, c.prev, c.perm)
	} else if err = os.Remove(c.path); errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		log.Printf("[api] cert: restore %s: %v", c.path, err)
	}
}

func (s *Server) handleDeleteCert(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	domain := r.PathValue("domain")

	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	if domain == "" || !validDomain.MatchString(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	domain, ok := resolveCertDomain(w, app, domain)
	if !ok {
		return
	}

	certDir := filepath.Join(filepath.Dir(app.ComposePath), "certs")
	certPath := filepath.Join(certDir, domain+".crt")
	keyPath := filepath.Join(certDir, domain+".key")

	os.Remove(certPath)
	os.Remove(keyPath)

	s.recordAudit(r, app, "endpoint", "cert_removed", map[string]any{"domain": domain}, nil)

	if !s.reloadProxyForCert(w, "deleted") {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
