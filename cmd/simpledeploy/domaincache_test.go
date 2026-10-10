package main

import (
	"errors"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newTestDomainCache(apps *[]store.App, calls *int, fail *bool) (*domainCache, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	c := newDomainCache(func() ([]store.App, error) {
		*calls++
		if fail != nil && *fail {
			return nil, errors.New("db down")
		}
		return append([]store.App(nil), (*apps)...), nil
	})
	c.now = clk.now
	return c, clk
}

func TestDomainCacheCachesLookups(t *testing.T) {
	apps := []store.App{{ID: 1, Domain: "a.example.com"}, {ID: 2, Domain: "B.Example.com"}}
	calls := 0
	c, clk := newTestDomainCache(&apps, &calls, nil)

	for i := 0; i < 100; i++ {
		id, err := c.Lookup("a.example.com:443")
		if err != nil || id != 1 {
			t.Fatalf("lookup = %d, %v", id, err)
		}
	}
	if id, err := c.Lookup("b.EXAMPLE.com"); err != nil || id != 2 {
		t.Fatalf("case-insensitive lookup = %d, %v", id, err)
	}
	if calls != 1 {
		t.Fatalf("ListApps called %d times, want 1", calls)
	}

	// Domain moves to another app: picked up after the TTL.
	apps[0].ID = 9
	clk.advance(domainCacheTTL - time.Second)
	if id, _ := c.Lookup("a.example.com"); id != 1 {
		t.Fatalf("before TTL got %d, want cached 1", id)
	}
	clk.advance(time.Second)
	if id, _ := c.Lookup("a.example.com"); id != 9 {
		t.Fatalf("after TTL got %d, want 9", id)
	}
}

func TestDomainCacheMissRefreshIsRateLimited(t *testing.T) {
	apps := []store.App{{ID: 1, Domain: "a.example.com"}}
	calls := 0
	c, clk := newTestDomainCache(&apps, &calls, nil)

	if _, err := c.Lookup("a.example.com"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if _, err := c.Lookup("unknown.example.com"); err == nil {
			t.Fatal("unknown domain resolved")
		}
	}
	if calls != 1 {
		t.Fatalf("misses within the refresh gap reloaded %d times", calls-1)
	}

	// A newly deployed app is found on demand once the miss gap elapsed.
	apps = append(apps, store.App{ID: 2, Domain: "new.example.com"})
	clk.advance(domainCacheMissRefresh)
	if id, err := c.Lookup("new.example.com"); err != nil || id != 2 {
		t.Fatalf("new domain = %d, %v", id, err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestDomainCacheIgnoresEmptyDomainsAndPicksLowestID(t *testing.T) {
	// ListApps orders by name, so the lowest ID may come last.
	apps := []store.App{{ID: 1, Domain: ""}, {ID: 5, Domain: "dup.example.com"}, {ID: 2, Domain: "Dup.example.com"}, {ID: 3, Domain: "dup.example.com"}}
	calls := 0
	c, _ := newTestDomainCache(&apps, &calls, nil)
	if _, err := c.Lookup(""); err == nil {
		t.Fatal("empty host matched an app without a domain")
	}
	if id, _ := c.Lookup("dup.example.com"); id != 2 {
		t.Fatalf("dup = %d, want lowest ID 2", id)
	}
}

func TestDomainCacheStaleOnError(t *testing.T) {
	apps := []store.App{{ID: 1, Domain: "a.example.com"}}
	calls := 0
	fail := true
	c, clk := newTestDomainCache(&apps, &calls, &fail)

	if _, err := c.Lookup("a.example.com"); err == nil || err.Error() != "db down" {
		t.Fatalf("err = %v, want db error", err)
	}
	fail = false
	clk.advance(domainCacheMissRefresh)
	if id, err := c.Lookup("a.example.com"); err != nil || id != 1 {
		t.Fatalf("after recovery = %d, %v", id, err)
	}
	fail = true
	clk.advance(domainCacheTTL)
	if id, err := c.Lookup("a.example.com"); err != nil || id != 1 {
		t.Fatalf("stale map not used on error: %d, %v", id, err)
	}
}
