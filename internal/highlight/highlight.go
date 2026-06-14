// Package highlight provides per-pane regex highlight profiles. Each profile
// is a named set of (pattern, CSS class) pairs. The browser SPA applies the
// CSS classes client-side; the server stores and serves the profiles via the
// REST API.
package highlight

import (
	"fmt"
	"regexp"
	"sync"
)

// Rule pairs a compiled regular expression with a CSS class name to apply to
// matching spans in the browser.
type Rule struct {
	Pattern string `json:"pattern"`
	Class   string `json:"class"`
}

// Profile is a named, ordered list of highlight rules.
type Profile struct {
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// Store holds named highlight profiles and is safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	profiles map[string]Profile
}

// New returns an empty Store.
func New() *Store {
	return &Store{profiles: make(map[string]Profile)}
}

// Add validates and stores a profile. Returns an error if any rule's pattern
// does not compile.
func (s *Store) Add(p Profile) error {
	if p.Name == "" {
		return fmt.Errorf("highlight: profile name must not be empty")
	}

	for i, r := range p.Rules {
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("highlight: profile %q rule %d pattern %q: %w", p.Name, i, r.Pattern, err)
		}

		if r.Class == "" {
			return fmt.Errorf("highlight: profile %q rule %d: class must not be empty", p.Name, i)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[p.Name] = p

	return nil
}

// Remove deletes the named profile. Returns an error if it does not exist.
func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.profiles[name]; !ok {
		return fmt.Errorf("highlight: profile %q not found", name)
	}

	delete(s.profiles, name)

	return nil
}

// Get returns the named profile and whether it was found.
func (s *Store) Get(name string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.profiles[name]

	return p, ok
}

// All returns every stored profile in unspecified order.
func (s *Store) All() []Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Profile, 0, len(s.profiles))
	for _, p := range s.profiles {
		out = append(out, p)
	}

	return out
}

// Len returns the number of stored profiles.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.profiles)
}
