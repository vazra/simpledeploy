package deployer

import (
	"encoding/base64"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxDetailLines = 3
	maxDetailBytes = 1000
)

// ComposeError is a failed docker compose command. Detail is the end of its
// error output, which says why it failed (image pull denied, port in use,
// ...), with registry credentials removed. It is what the deploy failure
// shows in the activity feed and the logs; the full output stays in the
// deploy log and deploy events.
type ComposeError struct {
	Op     string // "compose up", "compose pull", ...
	Detail string
	Err    error
}

func (e *ComposeError) Error() string {
	if e.Detail == "" {
		return e.Op + ": " + e.Err.Error()
	}
	return e.Op + ": " + e.Detail + ": " + e.Err.Error()
}

func (e *ComposeError) Unwrap() error { return e.Err }

func composeError(op string, err error, stderr string, auths []RegistryAuth) error {
	return &ComposeError{Op: op, Detail: failureDetail(redactAuths(stderr, auths)), Err: err}
}

// redactAuths removes registry credentials from command output. They only
// reach docker through the temporary config file, never args or stdin, so
// this is defense in depth.
func redactAuths(s string, auths []RegistryAuth) string {
	for _, a := range auths {
		if a.Password == "" {
			continue
		}
		s = strings.ReplaceAll(s, base64.StdEncoding.EncodeToString([]byte(a.Username+":"+a.Password)), "[redacted]")
		s = strings.ReplaceAll(s, a.Password, "[redacted]")
	}
	return s
}

// noiseLine matches docker compose progress lines ("Container x  Started",
// "web Pulling", "a1b2c3 Downloading ...") and warnings: they never say why
// a command failed.
var noiseLine = regexp.MustCompile(`^(?:(?:Container|Network|Volume|Image)\s+\S+\s+\S+(?:\s+[\d.]+s)?$|\S+\s+(?:Pulling|Pulled|Waiting|Downloading|Download complete|Extracting|Verifying Checksum|Pull complete|Already exists|Skipped|Interrupted)\b|WARN\[|.*\blevel=warning\b)`)

// failureDetail picks the lines of a failed compose command's stderr that
// say why it failed. Compose prints its error last, so these are the last
// few lines that are not progress or warnings, without repeats, capped at
// maxDetailBytes (keeping the end).
func failureDetail(stderr string) string {
	lines := strings.Split(stderr, "\n")
	var picked []string
	for i := len(lines) - 1; i >= 0 && len(picked) < maxDetailLines; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || noiseLine.MatchString(l) || restates(l, picked) {
			continue
		}
		picked = append(picked, l)
	}
	slices.Reverse(picked)
	detail := strings.Join(picked, "\n")
	if len(detail) > maxDetailBytes {
		cut := len(detail) - maxDetailBytes
		for cut < len(detail) && !utf8.RuneStart(detail[cut]) {
			cut++
		}
		detail = "..." + detail[cut:]
	}
	return detail
}

// restates reports whether line repeats an error already picked: a failed
// pull prints "web Error <msg>" and then "Error response from daemon: <msg>".
func restates(line string, picked []string) bool {
	msg := errorMessage(line)
	for _, p := range picked {
		if strings.Contains(errorMessage(p), msg) {
			return true
		}
	}
	return false
}

// errorMessage is line lowercased, with spacing collapsed and a leading
// "Error response from daemon:", "Error:" or "<service> Error" removed.
func errorMessage(line string) string {
	f := strings.Fields(strings.ToLower(line))
	s := strings.Join(f, " ")
	for _, p := range []string{"error response from daemon: ", "error: "} {
		if strings.HasPrefix(s, p) {
			return s[len(p):]
		}
	}
	if len(f) > 2 && f[1] == "error" {
		return strings.Join(f[2:], " ")
	}
	return s
}
