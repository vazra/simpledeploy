package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Cross-origin browser requests carrying the session cookie must be
// rejected even when the origin is same-site (sibling subdomain).
func TestCrossOriginProtectionRejectsSameSitePost(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginAs(t, srv, st, "boss", "password1", "super_admin")

	body := `{"username":"x","password":"xxxxxxxx","role":"viewer"}`
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"sec-fetch-site same-site", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"sec-fetch-site cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"origin mismatch", map[string]string{"Origin": "https://app.example.com"}, http.StatusForbidden},
		{"same-origin browser", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusCreated},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := strings.Replace(body, `"x"`, `"u`+string(rune('a'+i))+`"`, 1)
			req := httptest.NewRequest(http.MethodPost, "http://manage.example.com/api/users", strings.NewReader(b))
			req.Header.Set("Content-Type", "text/plain")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			req.AddCookie(cookie)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// Non-browser clients (CLI, curl, git webhooks) send no Origin or
// Sec-Fetch-Site and must keep working.
func TestCrossOriginProtectionAllowsNonBrowserClients(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginAs(t, srv, st, "boss", "password1", "super_admin")
	req := httptest.NewRequest(http.MethodPost, "/api/users",
		strings.NewReader(`{"username":"cli","password":"xxxxxxxx","role":"viewer"}`))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}

	// GET is never blocked, even cross-site.
	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", w.Code)
	}
}

func TestValidateComposeRequiresSuperAdmin(t *testing.T) {
	srv, st := newTestServer(t)
	viewer := loginAs(t, srv, st, "v", "password1", "viewer")
	manage := loginAs(t, srv, st, "m", "password1", "manage")
	admin := loginAs(t, srv, st, "a", "password1", "super_admin")
	payload := `{"compose":"` + base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n")) + `"}`
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"viewer", viewer, http.StatusForbidden},
		{"manage", manage, http.StatusForbidden},
		{"super_admin", admin, http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/apps/validate-compose", strings.NewReader(payload))
		req.AddCookie(tc.cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, w.Code, tc.want)
		}
	}
}

func TestHSTSBehindTrustedProxy(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetTrustedProxies([]string{"127.0.0.1"})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("expected HSTS from trusted proxy with X-Forwarded-Proto=https")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("untrusted peer must not be able to trigger HSTS via header")
	}
}

// Older browsers without Sec-Fetch-Site behind a Host-rewriting proxy are
// accepted when their Origin is the configured dashboard domain.
func TestCrossOriginProtectionTrustsDashboardDomain(t *testing.T) {
	srv, st := newTestServer(t)
	srv.SetReservedDomains("manage.example.com")
	cookie := loginAs(t, srv, st, "boss", "password1", "super_admin")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8443/api/users",
		strings.NewReader(`{"username":"viaproxy","password":"xxxxxxxx","role":"viewer"}`))
	req.Header.Set("Origin", "https://manage.example.com")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %q)", w.Code, w.Body.String())
	}
}

// The dashboard domain's http:// origin is trusted only when TLS is off;
// https:// is trusted in every mode.
func TestCrossOriginProtectionHTTPOriginOnlyWhenTLSOff(t *testing.T) {
	cases := []struct {
		tlsMode, origin string
		want            int
	}{
		{"auto", "http://manage.example.com", http.StatusForbidden},
		{"local", "http://manage.example.com", http.StatusForbidden},
		{"custom", "http://manage.example.com", http.StatusForbidden},
		{"", "http://manage.example.com", http.StatusForbidden},
		{"off", "http://manage.example.com", http.StatusCreated},
		{"auto", "https://manage.example.com", http.StatusCreated},
		{"off", "https://manage.example.com", http.StatusCreated},
	}
	for i, tc := range cases {
		t.Run(tc.tlsMode+" "+tc.origin, func(t *testing.T) {
			srv, st := newTestServer(t)
			srv.SetReservedDomains("manage.example.com")
			srv.SetTLSMode(tc.tlsMode)
			cookie := loginAs(t, srv, st, "boss", "password1", "super_admin")
			body := fmt.Sprintf(`{"username":"origin%d","password":"xxxxxxxx","role":"viewer"}`, i)
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8443/api/users", strings.NewReader(body))
			req.Header.Set("Origin", tc.origin)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
