package recipes

import (
	"context"
	"sync"
	"time"
)

type Cache struct {
	client *Client
	ttl    time.Duration

	mu      sync.Mutex
	idx     *Index
	fetched time.Time
}

func NewCache(c *Client, ttl time.Duration) *Cache {
	if ttl == 0 {
		ttl = 10 * time.Minute
	}
	return &Cache{client: c, ttl: ttl}
}

// Client returns the underlying HTTP client (used by API handlers to fetch
// per-recipe sub-resources without re-implementing host whitelisting).
func (c *Cache) Client() *Client { return c.client }

func (c *Cache) Index(ctx context.Context) (*Index, error) {
	c.mu.Lock()
	if c.idx != nil && time.Since(c.fetched) < c.ttl {
		idx := c.idx
		c.mu.Unlock()
		return idx, nil
	}
	c.mu.Unlock()

	idx, err := c.client.FetchIndex(ctx)
	if err != nil {
		c.mu.Lock()
		if c.idx != nil {
			stale := c.idx
			c.mu.Unlock()
			return stale, nil
		}
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Lock()
	c.idx = idx
	c.fetched = time.Now()
	c.mu.Unlock()
	return idx, nil
}

func (c *Cache) Invalidate() {
	c.mu.Lock()
	c.idx = nil
	c.mu.Unlock()
}
