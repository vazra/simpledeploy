// Package mirror rewrites docker.io image references in compose YAML to
// point at an alternate registry (typically a GHCR mirror). Intended for
// CI/E2E where Docker Hub rate limits can block deploys, but the same
// mechanism works for local dev when SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX
// is set.
package mirror

import (
	"regexp"
	"strings"
)

// imageLineRe matches a compose "image: value" line. Value may be quoted
// (single or double) and may have a trailing inline comment. Indentation
// is preserved by capture group 1.
var imageLineRe = regexp.MustCompile(`(?m)^([ \t]+image:[ \t]*)(['"]?)([^'"#\n]+?)(['"]?)([ \t]*(?:#.*)?)$`)

// RewriteCompose returns composeYAML with every docker.io-bound image
// reference rewritten to prefix + normalized path. prefix must include
// trailing slash (e.g. "ghcr.io/vazra/"). If prefix is empty, returns
// the input unchanged.
//
// Rules:
//   - Images already targeting a non-default registry (ghcr.io, quay.io,
//     any host with "." or ":" in the first path segment) are left alone.
//   - Official images (no namespace) get "library/" stripped; a single
//     segment "nginx:alpine" becomes "<prefix>nginx:alpine".
//   - Digest refs (@sha256:...) and tags (:tag) are preserved verbatim.
func RewriteCompose(composeYAML []byte, prefix string) []byte {
	if prefix == "" {
		return composeYAML
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	out := imageLineRe.ReplaceAllFunc(composeYAML, func(line []byte) []byte {
		m := imageLineRe.FindSubmatch(line)
		if m == nil {
			return line
		}
		img := strings.TrimSpace(string(m[3]))
		rewritten, ok := rewriteRef(img, prefix)
		if !ok {
			return line
		}
		// Preserve indentation + original quoting + trailing comment.
		return []byte(string(m[1]) + string(m[2]) + rewritten + string(m[4]) + string(m[5]))
	})
	return out
}

// rewriteRef returns the rewritten image ref and true, or ("", false)
// if the ref should not be rewritten.
func rewriteRef(ref, prefix string) (string, bool) {
	if ref == "" {
		return "", false
	}
	// Already pointing at the mirror - idempotent skip.
	if strings.HasPrefix(ref, prefix) {
		return "", false
	}
	// Split off registry host (if any). Docker image ref grammar:
	// [REGISTRY/]NAMESPACE/REPO[:TAG|@DIGEST]
	// A registry host is identifiable by containing "." or ":" before
	// the first "/", or being the literal "localhost".
	first, rest, hasSlash := strings.Cut(ref, "/")
	if hasSlash && isRegistryHost(first) {
		// Already has an explicit non-docker.io registry.
		if first == "docker.io" {
			// docker.io/library/nginx:tag -> <prefix>nginx:tag
			// docker.io/foo/bar:tag      -> <prefix>foo/bar:tag
			rest = strings.TrimPrefix(rest, "library/")
			return prefix + rest, true
		}
		return "", false
	}
	// No registry host - defaults to docker.io.
	// "nginx:tag" (no slash) -> prefix + ref
	// "foo/bar:tag" (has slash, first segment is not a host) -> prefix + ref
	_ = first
	return prefix + ref, true
}

// isRegistryHost reports whether s looks like a registry hostname as
// opposed to an image namespace. Per Docker's own heuristic: a host
// contains "." or ":" or is exactly "localhost".
func isRegistryHost(s string) bool {
	if s == "localhost" {
		return true
	}
	return strings.ContainsAny(s, ".:")
}
