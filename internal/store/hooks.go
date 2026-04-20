package store

import (
	"log"
	"sync"
)

// MutationScope describes what kind of entity changed.
type MutationScope int

const (
	ScopeGlobal MutationScope = iota
	ScopeApp
)

// MutationHook is called after a successful mutating DB operation.
// scope is Global for cross-app entities, App for per-app entities.
// slug is the app slug when scope is App, otherwise empty.
// Implementations must not block (callers may hold locks).
type MutationHook func(scope MutationScope, slug string)

type hookState struct {
	mu   sync.Mutex
	hook MutationHook
}

// SetMutationHook installs a callback. Nil disables it. Safe to change at runtime.
func (s *Store) SetMutationHook(h MutationHook) {
	s.hooks.mu.Lock()
	s.hooks.hook = h
	s.hooks.mu.Unlock()
}

func (s *Store) fireHook(scope MutationScope, slug string) {
	s.hooks.mu.Lock()
	h := s.hooks.hook
	s.hooks.mu.Unlock()
	if h == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[store] mutation hook panic: %v", r)
		}
	}()
	h(scope, slug)
}

// fireAppHook resolves slug from app ID and fires ScopeApp. Silent if the lookup fails.
func (s *Store) fireAppHook(appID int64) {
	app, err := s.GetAppByID(appID)
	if err != nil || app == nil {
		return
	}
	s.fireHook(ScopeApp, app.Slug)
}
