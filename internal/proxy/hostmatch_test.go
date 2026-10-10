package proxy

import "testing"

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"Example.COM":       "example.com",
		"example.com:8443":  "example.com",
		"EXAMPLE.com.":      "example.com",
		"Example.com.:443":  "example.com",
		"*.Example.com":     "*.example.com",
		"[::1]:443":         "::1",
		"[::1]":             "::1",
		" padded.example ":  "padded.example",
		"":                  "",
		"localhost:8080":    "localhost",
		"10.0.0.1:80":       "10.0.0.1",
		"Sub.Domain.Test":   "sub.domain.test",
		"sub.domain.test..": "sub.domain.test.",
	}
	for in, want := range cases {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostMatches(t *testing.T) {
	cases := []struct {
		pattern, host string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "www.example.com", false},
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "a.b.example.com", false}, // one label per "*", like Caddy
		{"*.*.example.com", "a.b.example.com", true},
		{"foo.*.com", "foo.example.com", true},
		{"foo.*.com", "bar.example.com", false},
		{"f*.example.com", "foo.example.com", false}, // partial-label "*" is literal in Caddy
	}
	for _, tc := range cases {
		if got := hostMatches(tc.pattern, tc.host); got != tc.want {
			t.Errorf("hostMatches(%q, %q) = %v, want %v", tc.pattern, tc.host, got, tc.want)
		}
	}
}

func TestLookupHostPrefersMostSpecific(t *testing.T) {
	m := map[string]string{
		"foo.example.com": "exact",
		"*.example.com":   "wild1",
		"*.*.com":         "wild2",
		"foo.*.com":       "wild1b",
	}
	cases := map[string]string{
		"FOO.example.com:443": "exact",
		"bar.example.com":     "wild1",  // 1 wildcard beats "*.*.com"
		"foo.other.com":       "wild1b", // 1 wildcard beats 2
		"bar.other.com":       "wild2",
	}
	for host, want := range cases {
		got, ok := lookupHost(m, host)
		if !ok || got != want {
			t.Errorf("lookupHost(%q) = %q, %v; want %q", host, got, ok, want)
		}
	}
	if _, ok := lookupHost(m, "example.org"); ok {
		t.Error("lookupHost(example.org) matched, want no match")
	}
}

func TestDomainLess(t *testing.T) {
	if !domainLess("z.example.com", "*.example.com") {
		t.Error("exact must sort before wildcard")
	}
	if !domainLess("*.foo.example.com", "*.*.example.com") {
		t.Error("fewer wildcard labels must sort first")
	}
	if !domainLess("*.a.com", "*.b.com") {
		t.Error("same wildcard count must sort lexically")
	}
}
