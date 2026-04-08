package proxy

import "sync"

// MockProxy is an in-memory Proxy for testing.
type MockProxy struct {
	mu      sync.Mutex
	routes  []Route
	running bool
}

// NewMockProxy creates a new MockProxy.
func NewMockProxy() *MockProxy {
	return &MockProxy{running: true}
}

// SetRoutes stores routes.
func (m *MockProxy) SetRoutes(routes []Route) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = make([]Route, len(routes))
	copy(m.routes, routes)
	return nil
}

// Stop marks the proxy as stopped.
func (m *MockProxy) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	return nil
}

// Routes returns a copy of the current routes.
func (m *MockProxy) Routes() []Route {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Route, len(m.routes))
	copy(out, m.routes)
	return out
}

// HasRoute reports whether a route with the given domain exists.
func (m *MockProxy) HasRoute(domain string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.routes {
		if r.Domain == domain {
			return true
		}
	}
	return false
}
