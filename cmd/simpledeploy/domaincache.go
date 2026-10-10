package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

const (
	// domainCacheTTL is how long the domain map is trusted before a full
	// reload.
	domainCacheTTL = 10 * time.Second
	// domainCacheMissRefresh is the minimum gap between reloads triggered
	// by a lookup miss, so a newly deployed app is attributed quickly while
	// a flood of unknown hosts cannot turn into one ListApps per request.
	domainCacheMissRefresh = 2 * time.Second
)

// domainCache maps lowercase app domains to app IDs for request-stats
// attribution. Every reload (scheduled, miss-triggered or after an error) is
// rate-limited by loaded, so a failing DB or a flood of unknown hosts costs
// at most one ListApps per domainCacheMissRefresh.
type domainCache struct {
	list func() ([]store.App, error)
	now  func() time.Time

	mu      sync.Mutex
	ids     map[string]int64
	loaded  time.Time // zero = never loaded
	lastErr error
}

func newDomainCache(list func() ([]store.App, error)) *domainCache {
	return &domainCache{list: list, now: time.Now}
}

// Lookup resolves a request host (optionally with :port) to an app ID.
func (c *domainCache) Lookup(domain string) (int64, error) {
	host, _, _ := strings.Cut(domain, ":")
	host = strings.ToLower(host)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.loaded.IsZero() || now.Sub(c.loaded) >= domainCacheTTL {
		c.reloadLocked(now)
	}
	if id, ok := c.ids[host]; ok && host != "" {
		return id, nil
	}
	if now.Sub(c.loaded) >= domainCacheMissRefresh {
		c.reloadLocked(now)
		if id, ok := c.ids[host]; ok && host != "" {
			return id, nil
		}
	}
	if c.ids == nil && c.lastErr != nil {
		return 0, c.lastErr
	}
	return 0, fmt.Errorf("unknown domain: %s", domain)
}

// reloadLocked rebuilds the map. On error the previous map (if any) stays
// in use.
func (c *domainCache) reloadLocked(now time.Time) {
	c.loaded = now
	apps, err := c.list()
	c.lastErr = err
	if err != nil {
		return
	}
	ids := make(map[string]int64, len(apps))
	for _, a := range apps {
		d := strings.ToLower(a.Domain)
		if d == "" {
			continue
		}
		// Lowest ID wins a shared domain, like proxy domain ownership.
		if cur, dup := ids[d]; !dup || a.ID < cur {
			ids[d] = a.ID
		}
	}
	c.ids = ids
}
