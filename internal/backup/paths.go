package backup

import (
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/robfig/cron/v3"
)

// sqlitePathForbidden lists characters refused in SQLite database paths.
// The path is an argv element (no shell), but its base name is embedded in
// a single-quoted sqlite3 '.backup' dot-command, where a quote would end
// the argument. Quotes, backslash, backtick and '$' are refused so the path
// stays safe for any quoting style; spaces, '@' and the like are fine.
const sqlitePathForbidden = "'\"\\`$"

// ValidatePath checks a container path that ends up as a tar/cp operand.
// It must be absolute, printable, in clean form apart from an optional
// trailing '/' (no '//', '.' or '..' parts), not "/" itself, and no element
// may start with '-' (so it can never be parsed as a command-line flag).
func ValidatePath(p string) error {
	if p == "" {
		return fmt.Errorf("backup path is empty")
	}
	if !utf8.ValidString(p) {
		return fmt.Errorf("backup path %q is not valid UTF-8", p)
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return fmt.Errorf("backup path %q contains control characters or newlines", p)
		}
	}
	if !path.IsAbs(p) {
		return fmt.Errorf("backup path %q must be absolute (start with /)", p)
	}
	if p == "/" {
		return fmt.Errorf("backup path \"/\" would cover the whole container; choose a specific directory such as /data")
	}
	// A trailing slash is harmless; other unclean forms ('//', '.', '..')
	// are refused so the archived path is unambiguous.
	if c := path.Clean(p); c != strings.TrimRight(p, "/") {
		return fmt.Errorf("backup path %q must be written without '//', '.' or '..' parts; use %q", p, c)
	}
	for _, el := range strings.Split(p, "/") {
		if strings.HasPrefix(el, "-") {
			return fmt.Errorf("backup path %q has a part starting with '-', which is not allowed", p)
		}
	}
	return nil
}

// ValidateSQLitePath applies ValidatePath, refuses quotes, backslash,
// backtick and '$', and requires a file name (no trailing '/').
func ValidateSQLitePath(p string) error {
	if err := ValidatePath(p); err != nil {
		return err
	}
	if strings.ContainsAny(p, sqlitePathForbidden) {
		return fmt.Errorf("SQLite database path %q may not contain quotes, backslashes, backticks or '$'", p)
	}
	if strings.HasSuffix(p, "/") {
		return fmt.Errorf("SQLite database path %q must point to a database file; remove the trailing '/'", p)
	}
	switch path.Base(p) {
	case "/", ".", "..":
		return fmt.Errorf("SQLite database path %q must point to a database file", p)
	}
	return nil
}

// ValidatePaths validates paths for the given strategy. SQLite paths get the
// extra character check.
func ValidatePaths(strategy string, paths []string) error {
	for _, p := range paths {
		var err error
		if strategy == "sqlite" {
			err = ValidateSQLitePath(p)
		} else {
			err = ValidatePath(p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidatePathsConfig parses a stored paths value (JSON array or
// comma-separated list, same as the scheduler) and validates it.
func ValidatePathsConfig(strategy, raw string) error {
	return ValidatePaths(strategy, parsePaths(raw))
}

// ValidateCron parses expr with the same parser the scheduler's cron runner
// uses. An empty expression (no schedule) is valid.
func ValidateCron(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("invalid backup schedule %q: %v. Use a 5-field cron expression such as \"0 2 * * *\" (every day at 2:00)", expr, err)
	}
	return nil
}

// validateRunPaths validates paths at run time for strategies that hand
// them to tar/cp. Other strategies ignore Paths.
func validateRunPaths(strategy string, paths []string) error {
	if strategy != "volume" && strategy != "sqlite" {
		return nil
	}
	return ValidatePaths(strategy, paths)
}
