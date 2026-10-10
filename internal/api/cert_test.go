package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertOnlyForAppEndpointDomains(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)
	writeEndpointApp(t, s, "other", "other.example.com")
	minePath := writeEndpointApp(t, s, "mine", "Mine.Example.com")
	certDir := filepath.Join(filepath.Dir(minePath), "certs")
	certPEM, keyPEM := genTestCertPEM(t)
	body := map[string]string{"cert": certPEM, "key": keyPEM}

	for _, domain := range []string{"other.example.com", "unknown.example.com"} {
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, authedRequest(t, http.MethodPut, "/api/apps/mine/certs/"+domain, body, cookie))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not one of this app's endpoints") {
			t.Errorf("upload %s: %d %q, want 400 not an endpoint", domain, w.Code, w.Body.String())
		}
		if _, err := os.Stat(filepath.Join(certDir, domain+".crt")); err == nil {
			t.Errorf("upload %s: cert written", domain)
		}
	}

	// Case-insensitive match; the file uses the endpoint's spelling so the
	// proxy finds it.
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, authedRequest(t, http.MethodPut, "/api/apps/mine/certs/mine.example.com", body, cookie))
	if w.Code != http.StatusOK {
		t.Fatalf("upload own domain: %d %q", w.Code, w.Body.String())
	}
	names := map[string]bool{}
	entries, _ := os.ReadDir(certDir)
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["Mine.Example.com.crt"] || !names["Mine.Example.com.key"] {
		t.Fatalf("cert files = %v, want Mine.Example.com.{crt,key}", names)
	}

	// Delete: refused for other domains, file of own domain kept until then.
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, authedRequest(t, http.MethodDelete, "/api/apps/mine/certs/other.example.com", nil, cookie))
	if w.Code != http.StatusBadRequest {
		t.Errorf("delete other domain: %d, want 400", w.Code)
	}
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, authedRequest(t, http.MethodDelete, "/api/apps/mine/certs/MINE.example.com", nil, cookie))
	if w.Code != http.StatusOK {
		t.Fatalf("delete own domain: %d %q", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(certDir, "Mine.Example.com.crt")); !os.IsNotExist(err) {
		t.Error("own cert not deleted")
	}
}

// genCertPEMFor returns a self-signed cert valid for hosts and its key.
func genCertPEMFor(t *testing.T, hosts ...string) (certPEM, keyPEM string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: hosts[0]},
		DNSNames:     hosts,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

func TestCertUploadVerifiesPairAndDomain(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)
	minePath := writeEndpointApp(t, s, "mine", "mine.example.com")
	certDir := filepath.Join(filepath.Dir(minePath), "certs")

	certA, _ := genCertPEMFor(t, "mine.example.com")
	_, keyB := genCertPEMFor(t, "mine.example.com")
	otherCert, otherKey := genCertPEMFor(t, "other.example.com")
	cases := []struct {
		name, cert, key, want string
	}{
		{"key of another cert", certA, keyB, "do not match"},
		{"cert for another domain", otherCert, otherKey, "not valid for mine.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, authedRequest(t, http.MethodPut, "/api/apps/mine/certs/mine.example.com",
				map[string]string{"cert": tc.cert, "key": tc.key}, cookie))
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("status = %d body = %q, want 400 %q", w.Code, w.Body.String(), tc.want)
			}
			if _, err := os.Stat(filepath.Join(certDir, "mine.example.com.crt")); err == nil {
				t.Fatal("cert written")
			}
		})
	}

	// A wildcard cert covers a subdomain endpoint.
	wildPath := writeEndpointApp(t, s, "wild", "api.wild.example.com")
	wc, wk := genCertPEMFor(t, "*.wild.example.com")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, authedRequest(t, http.MethodPut, "/api/apps/wild/certs/api.wild.example.com",
		map[string]string{"cert": wc, "key": wk}, cookie))
	if w.Code != http.StatusOK {
		t.Fatalf("wildcard cert: status = %d body = %q", w.Code, w.Body.String())
	}
	if fi, err := os.Lstat(filepath.Join(filepath.Dir(wildPath), "certs", "api.wild.example.com.key")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v, want 0600", fi, err)
	}
}

func TestCertUploadRollsBackOnReloadFailure(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)
	srv.SetProxyReloader(&countingReloader{err: errors.New("boom")})
	minePath := writeEndpointApp(t, s, "mine", "mine.example.com", "fresh.example.com")
	certDir := filepath.Join(filepath.Dir(minePath), "certs")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(certDir, "mine.example.com.crt"), []byte("OLD-CERT"), 0o644)
	os.WriteFile(filepath.Join(certDir, "mine.example.com.key"), []byte("OLD-KEY"), 0o600)

	upload := func(domain string) {
		t.Helper()
		c, k := genCertPEMFor(t, domain)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, authedRequest(t, http.MethodPut, "/api/apps/mine/certs/"+domain,
			map[string]string{"cert": c, "key": k}, cookie))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("%s: status = %d, want 500; body: %s", domain, w.Code, w.Body.String())
		}
	}

	// Previous files are put back.
	upload("mine.example.com")
	if got, _ := os.ReadFile(filepath.Join(certDir, "mine.example.com.crt")); string(got) != "OLD-CERT" {
		t.Errorf("cert = %q, want previous content", got)
	}
	if got, _ := os.ReadFile(filepath.Join(certDir, "mine.example.com.key")); string(got) != "OLD-KEY" {
		t.Errorf("key = %q, want previous content", got)
	}

	// New files without a previous version are removed.
	upload("fresh.example.com")
	for _, name := range []string{"fresh.example.com.crt", "fresh.example.com.key"} {
		if _, err := os.Stat(filepath.Join(certDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s left behind: %v", name, err)
		}
	}
	if e := findFullAuditEntry(t, s, "endpoint", "cert_uploaded"); e != nil {
		t.Error("cert_uploaded audited although the upload was rolled back")
	}
}
