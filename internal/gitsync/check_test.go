package gitsync

import (
	"errors"
	"net"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

func TestClassifyRemoteErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "ok"},
		{"auth required", transport.ErrAuthenticationRequired, "auth_failed"},
		{"auth forbidden", transport.ErrAuthorizationFailed, "auth_failed"},
		{"not found", transport.ErrRepositoryNotFound, "not_found"},
		{"net dns", &net.DNSError{Err: "no such host", Name: "x"}, "network"},
		{"unknown", errors.New("boom"), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyRemoteErr(tc.err)
			if got != tc.want {
				t.Fatalf("classifyRemoteErr(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
