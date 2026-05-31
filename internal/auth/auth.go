// Package auth implements token-based access control for rdw.
//
// Tokens are stored hashed (SHA-256). The plain-text token is returned once
// on creation and never stored.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const tokenBytes = 32 // 256-bit entropy

// Token is an access token record. The Hash field is the SHA-256 of the
// plain-text secret; the secret itself is never stored.
type Token struct {
	ID        string
	Hash      string
	CreatedAt time.Time
	ExpiresAt time.Time // zero value means no expiry
	Panes     []string  // empty means all panes
	Windows   []string  // empty means all windows
}

// IsExpired reports whether the token has passed its expiry time.
func (t Token) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}

	return time.Now().After(t.ExpiresAt)
}

// HasPaneAccess reports whether the token grants access to the given pane ID.
// If the token's Panes list is empty, all panes are permitted.
func (t Token) HasPaneAccess(paneID string) bool {
	if len(t.Panes) == 0 {
		return true
	}

	for _, p := range t.Panes {
		if p == paneID {
			return true
		}
	}

	return false
}

// Store is a thread-safe registry of access tokens.
type Store struct {
	mu     sync.RWMutex
	tokens map[string]*Token // keyed by token ID
}

// NewStore creates an empty token store.
func NewStore() *Store {
	return &Store{
		tokens: make(map[string]*Token),
	}
}

// CreateOptions holds parameters for token creation.
type CreateOptions struct {
	Expiry  time.Duration // 0 means no expiry; default enforced by caller
	Panes   []string
	Windows []string
}

// Create generates a new token, stores its hash, and returns the plain-text
// secret. The secret is not stored and cannot be recovered after this call.
func (s *Store) Create(opts CreateOptions) (plaintext string, tok Token, err error) {
	raw := make([]byte, tokenBytes)

	if _, err = rand.Read(raw); err != nil {
		return "", Token{}, fmt.Errorf("generating token entropy: %w", err)
	}

	plain := hex.EncodeToString(raw)
	hash := sha256Hex(plain)
	id := hash[:16] // first 16 hex chars as the public ID

	now := time.Now().UTC()
	tok = Token{
		ID:        id,
		Hash:      hash,
		CreatedAt: now,
		Panes:     opts.Panes,
		Windows:   opts.Windows,
	}

	if opts.Expiry > 0 {
		tok.ExpiresAt = now.Add(opts.Expiry)
	}

	s.mu.Lock()
	s.tokens[id] = &tok
	s.mu.Unlock()

	return plain, tok, nil
}

// Verify checks the plain-text token against the stored hash. Returns the
// Token and true if valid and not expired, otherwise returns false.
func (s *Store) Verify(plaintext string) (Token, bool) {
	hash := sha256Hex(plaintext)
	id := hash[:16]

	s.mu.RLock()
	tok, ok := s.tokens[id]
	s.mu.RUnlock()

	if !ok {
		return Token{}, false
	}

	if tok.Hash != hash {
		return Token{}, false
	}

	if tok.IsExpired() {
		return Token{}, false
	}

	return *tok, true
}

// Revoke removes a token by ID. All active connections bound to this token
// must be terminated by the caller after Revoke returns.
func (s *Store) Revoke(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.tokens[id]
	delete(s.tokens, id)

	return ok
}

// List returns a snapshot of all non-expired tokens.
func (s *Store) List() []Token {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Token, 0, len(s.tokens))

	for _, t := range s.tokens {
		if !t.IsExpired() {
			out = append(out, *t)
		}
	}

	return out
}

// Len returns the total number of tokens including expired ones.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.tokens)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
