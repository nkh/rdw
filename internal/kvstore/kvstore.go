// Package kvstore implements the session-scoped key-value store.
package kvstore

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

const (
	KeyMaxLen        = 64
	ValueMaxBytes    = 64 * 1024       // 64 KB per value
	StoreMaxBytes    = 64 * 1024 * 1024 // 64 MB total
)

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_ :-]*$`)

// Key is a validated KV store key.
type Key string

// ParseKey validates and returns a Key or an error.
func ParseKey(s string) (Key, error) {
	if len(s) == 0 {
		return "", fmt.Errorf("key must not be empty")
	}

	if len(s) > KeyMaxLen {
		return "", fmt.Errorf("key %q exceeds maximum length of %d characters", s, KeyMaxLen)
	}

	if !keyPattern.MatchString(s) {
		return "", fmt.Errorf("key %q contains invalid characters: must match [a-zA-Z0-9_][a-zA-Z0-9_ :-]*", s)
	}

	return Key(s), nil
}

// String returns the string form of the key.
func (k Key) String() string { return string(k) }

// Store is a thread-safe, session-scoped key-value store.
type Store struct {
	mu         sync.RWMutex
	data       map[Key]string
	totalBytes int
}

// New creates an empty Store.
func New() *Store {
	return &Store{
		data: make(map[Key]string),
	}
}

// Set writes a value for the given key. Returns an error if the value exceeds
// per-entry or total store size limits.
func (s *Store) Set(k Key, value string) error {
	if len(value) > ValueMaxBytes {
		return fmt.Errorf("value for key %q exceeds maximum size of %d bytes", k, ValueMaxBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing := len(s.data[k])
	delta := len(value) - existing

	if s.totalBytes+delta > StoreMaxBytes {
		return fmt.Errorf("store size limit of %d bytes would be exceeded", StoreMaxBytes)
	}

	s.data[k] = value
	s.totalBytes += delta

	return nil
}

// Get retrieves the value for key. Returns ("", false) if the key is absent.
func (s *Store) Get(k Key) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[k]

	return v, ok
}

// Delete removes a key. No-op if the key does not exist.
func (s *Store) Delete(k Key) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.data[k]; ok {
		s.totalBytes -= len(v)
		delete(s.data, k)
	}
}

// Keys returns all keys with the optional prefix, sorted alphabetically.
func (s *Store) Keys(prefix string) []Key {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Key, 0, len(s.data))

	for k := range s.data {
		if prefix == "" || strings.HasPrefix(k.String(), prefix) {
			result = append(result, k)
		}
	}

	sortKeys(result)

	return result
}

// Snapshot returns a shallow copy of all key-value pairs.
func (s *Store) Snapshot() map[Key]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[Key]string, len(s.data))

	for k, v := range s.data {
		out[k] = v
	}

	return out
}

// TotalBytes returns the current total byte usage of all stored values.
func (s *Store) TotalBytes() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.totalBytes
}

// Len returns the number of keys currently stored.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.data)
}
