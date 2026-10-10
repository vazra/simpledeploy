package backup

import (
	"strings"
	"testing"
)

func TestValidatePath(t *testing.T) {
	good := []string{"/data", "/var/lib/postgresql/data", "/app/my data", "/app/a-b_c.d", "/srv/x-"}
	for _, p := range good {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil", p, err)
		}
	}
	bad := []string{
		"",
		"data",          // relative
		"./data",        // relative
		"/data\nrm",     // newline
		"/data\x00",     // NUL
		"/data\t",       // control
		"/-rf",          // flag-like element
		"/data/--files", // flag-like element
		"/a/\x7f",       // DEL
		"/a/\xff",       // invalid UTF-8
		"/",             // whole filesystem
		"//",            // whole filesystem, unclean
		"/data//x",      // double slash
		"/data/./x",     // dot element
		"/data/../etc",  // parent element
		"/data/..",      // parent element
		"/.",            // dot
	}
	for _, p := range bad {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) = nil, want error", p)
		}
	}
}

func TestValidatePath_UncleanSuggestsCleanForm(t *testing.T) {
	err := ValidatePath("/data/./x")
	if err == nil || !strings.Contains(err.Error(), `"/data/x"`) {
		t.Fatalf("ValidatePath(/data/./x) = %v, want error suggesting \"/data/x\"", err)
	}
	if err := ValidatePath("/data/"); err != nil {
		t.Fatalf("trailing slash must be accepted: %v", err)
	}
}

func TestValidateSQLitePath(t *testing.T) {
	good := []string{
		"/data/app.db",
		"/var/lib/app/db.sqlite3",
		"/data/A_b-c.1.db",
		"/data/app db.sqlite",    // space
		"/srv/My Apps/data/x.db", // spaces in directories
		"/data/user@host.db",     // '@'
		"/data/@scope/app.db",    // '@' element
		"/data/café (old)+1.db",  // non-ASCII, parens, plus
	}
	for _, p := range good {
		if err := ValidateSQLitePath(p); err != nil {
			t.Errorf("ValidateSQLitePath(%q) = %v, want nil", p, err)
		}
	}
	bad := []string{
		"/data/app'.db",  // quote would break the .backup command
		"/data/app\".db", // double quote
		"/data/a\\b.db",  // backslash
		"/data/`id`.db",  // backtick
		"/data/$(id).db", // shell chars
		"/data/$HOME.db",
		"/data/app\n.db", // newline
		"/data/app\t.db", // tab
		"/data/-x.db",    // flag-like
		"/-data/x.db",    // flag-like element
		"data/app.db",    // relative
		"/",              // no file name
		"/data/..",       // no file name
		"/data/app.db/",  // trailing slash names a folder
		"/data/\xff.db",  // invalid UTF-8
	}
	for _, p := range bad {
		if err := ValidateSQLitePath(p); err == nil {
			t.Errorf("ValidateSQLitePath(%q) = nil, want error", p)
		}
	}
}

func TestSQLiteBackupDotCmd(t *testing.T) {
	cases := map[string]string{
		"/data/app.db":           ".backup '/tmp/sd-backup-app.db'",
		"/srv/My Apps/app db.db": ".backup '/tmp/sd-backup-app db.db'",
		"/data/user@host.db":     ".backup '/tmp/sd-backup-user@host.db'",
	}
	for dbPath, want := range cases {
		if err := ValidateSQLitePath(dbPath); err != nil {
			t.Fatalf("ValidateSQLitePath(%q) = %v", dbPath, err)
		}
		if got := sqliteBackupDotCmd(sqliteTmpPath(dbPath)); got != want {
			t.Errorf("dot-command for %q = %q, want %q", dbPath, got, want)
		}
	}
}

func TestValidatePathsConfig(t *testing.T) {
	cases := []struct {
		strategy, raw string
		wantErr       bool
	}{
		{"volume", "", false},
		{"volume", "null", false},
		{"volume", `["/data","/config"]`, false},
		{"volume", "/data, /config", false},
		{"volume", `["relative"]`, true},
		{"volume", `["/data","/-x"]`, true},
		{"volume", "/data,-x", true},
		{"sqlite", `["/data/app.db"]`, false},
		{"sqlite", `["/data/app db.sqlite"]`, false},
		{"sqlite", `["/data/app'.sqlite"]`, true},
		{"sqlite", `["/data/app.db/"]`, true},
		{"volume", `["/data/"]`, false},
		{"postgres", `["/var/lib/postgresql/data"]`, false},
	}
	for _, tc := range cases {
		err := ValidatePathsConfig(tc.strategy, tc.raw)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidatePathsConfig(%q, %q) = %v, wantErr %v", tc.strategy, tc.raw, err, tc.wantErr)
		}
	}
}

func TestValidateCron(t *testing.T) {
	for _, expr := range []string{"", "0 2 * * *", "*/15 * * * *", "@daily", "@every 1m", "CRON_TZ=UTC 0 3 * * 1-5"} {
		if err := ValidateCron(expr); err != nil {
			t.Errorf("ValidateCron(%q) = %v, want nil", expr, err)
		}
	}
	for _, expr := range []string{"not a cron", "0 2 * *", "61 * * * *", "* * * * * *", "@sometimes"} {
		err := ValidateCron(expr)
		if err == nil {
			t.Errorf("ValidateCron(%q) = nil, want error", expr)
			continue
		}
		if !strings.Contains(err.Error(), "invalid backup schedule") {
			t.Errorf("ValidateCron(%q) error not human readable: %v", expr, err)
		}
	}
}

func TestTarCreateArgs_EndOfOptions(t *testing.T) {
	args := tarCreateArgs("c1", []string{"/data", "/srv/files"})
	want := []string{"exec", "c1", "tar", "-czf", "-", "-C", "/", "--", "data", "srv/files"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("tarCreateArgs = %q, want %q", args, want)
	}
}
