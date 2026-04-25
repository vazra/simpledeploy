package audit

import (
	"context"
	"testing"
)

func TestPrunerRespectsRetention(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.DB().Exec(`INSERT INTO audit_log (actor_source, category, action, summary, created_at) VALUES ('system','compose','changed','x', datetime('now','-400 day'))`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAuditRetentionDays(ctx, 365); err != nil {
		t.Fatal(err)
	}

	p := NewPruner(s, 0)
	if err := p.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n)
	if n != 0 {
		t.Errorf("expected 0 after prune, got %d", n)
	}
}

func TestPrunerNoopWhenDisabled(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.DB().Exec(`INSERT INTO audit_log (actor_source, category, action, summary, created_at) VALUES ('system','compose','changed','x', datetime('now','-400 day'))`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAuditRetentionDays(ctx, 0); err != nil {
		t.Fatal(err)
	}

	p := NewPruner(s, 0)
	if err := p.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n)
	if n != 1 {
		t.Errorf("expected 1 (no prune), got %d", n)
	}
}
