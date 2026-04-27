package store

import "testing"

func TestSystemMeta_RoundTrip(t *testing.T) {
	s := newTestStore(t)

	if _, ok, err := s.GetMeta("missing"); err != nil || ok {
		t.Fatalf("GetMeta missing: ok=%v err=%v", ok, err)
	}

	if err := s.SetMeta("foo", "bar"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	v, ok, err := s.GetMeta("foo")
	if err != nil || !ok || v != "bar" {
		t.Fatalf("GetMeta foo: v=%q ok=%v err=%v", v, ok, err)
	}

	if err := s.SetMeta("foo", "baz"); err != nil {
		t.Fatalf("SetMeta upsert: %v", err)
	}
	v, ok, err = s.GetMeta("foo")
	if err != nil || !ok || v != "baz" {
		t.Fatalf("GetMeta foo upserted: v=%q ok=%v err=%v", v, ok, err)
	}
}

func TestBackfillBackupConfigUUIDs_AssignsForNullsOnly(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)

	const N = 3
	for i := 0; i < N; i++ {
		// raw insert without uuid to simulate pre-migration rows.
		if _, err := s.db.Exec(`
			INSERT INTO backup_configs (app_id, strategy, target, schedule_cron, target_config_json,
				retention_mode, retention_count, verify_upload, pre_hooks, post_hooks, paths)
			VALUES (?, 'postgres', 'local', '0 * * * *', '{}', 'count', 3, 0, '', '', '')
		`, app.ID); err != nil {
			t.Fatalf("raw insert: %v", err)
		}
	}

	n, err := s.BackfillBackupConfigUUIDs()
	if err != nil {
		t.Fatalf("BackfillBackupConfigUUIDs: %v", err)
	}
	if n != N {
		t.Fatalf("backfill count = %d, want %d", n, N)
	}

	rows, err := s.db.Query(`SELECT uuid FROM backup_configs`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if u == "" {
			t.Fatalf("empty uuid after backfill")
		}
		if seen[u] {
			t.Fatalf("duplicate uuid %q", u)
		}
		seen[u] = true
	}

	n2, err := s.BackfillBackupConfigUUIDs()
	if err != nil {
		t.Fatalf("BackfillBackupConfigUUIDs second: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second backfill count = %d, want 0", n2)
	}
}
