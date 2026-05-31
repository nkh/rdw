package auth_test

import (
	"testing"
	"time"

	"github.com/nkh/rdw/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate_ReturnsPlaintext(t *testing.T) {
	s := auth.NewStore()
	plain, tok, err := s.Create(auth.CreateOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, plain)
	assert.NotEmpty(t, tok.ID)
	assert.NotEmpty(t, tok.Hash)
	// hash must not equal plain
	assert.NotEqual(t, plain, tok.Hash)
}

func TestCreate_HashIsNotPlaintext(t *testing.T) {
	s := auth.NewStore()
	plain, tok, err := s.Create(auth.CreateOptions{})
	require.NoError(t, err)
	assert.NotEqual(t, plain, tok.Hash)
}

func TestVerify_ValidToken(t *testing.T) {
	s := auth.NewStore()
	plain, _, err := s.Create(auth.CreateOptions{})
	require.NoError(t, err)

	got, ok := s.Verify(plain)
	assert.True(t, ok)
	assert.NotEmpty(t, got.ID)
}

func TestVerify_WrongSecret(t *testing.T) {
	s := auth.NewStore()
	_, _, err := s.Create(auth.CreateOptions{})
	require.NoError(t, err)

	_, ok := s.Verify("wrongsecret")
	assert.False(t, ok)
}

func TestVerify_ExpiredToken(t *testing.T) {
	s := auth.NewStore()
	// Create a token that expired 1 millisecond ago
	plain, _, err := s.Create(auth.CreateOptions{Expiry: time.Millisecond})
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	_, ok := s.Verify(plain)
	assert.False(t, ok)
}

func TestVerify_NoExpiry(t *testing.T) {
	s := auth.NewStore()
	plain, tok, err := s.Create(auth.CreateOptions{Expiry: 0})
	require.NoError(t, err)

	assert.True(t, tok.ExpiresAt.IsZero())

	_, ok := s.Verify(plain)
	assert.True(t, ok)
}

func TestRevoke_RemovesToken(t *testing.T) {
	s := auth.NewStore()
	plain, tok, err := s.Create(auth.CreateOptions{})
	require.NoError(t, err)

	revoked := s.Revoke(tok.ID)
	assert.True(t, revoked)

	_, ok := s.Verify(plain)
	assert.False(t, ok)
}

func TestRevoke_MissingToken(t *testing.T) {
	s := auth.NewStore()
	revoked := s.Revoke("nonexistent")
	assert.False(t, revoked)
}

func TestList_ExcludesExpired(t *testing.T) {
	s := auth.NewStore()

	_, _, err := s.Create(auth.CreateOptions{Expiry: time.Millisecond})
	require.NoError(t, err)
	_, _, err = s.Create(auth.CreateOptions{Expiry: time.Hour})
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	tokens := s.List()
	assert.Len(t, tokens, 1)
}

func TestToken_HasPaneAccess_EmptyMeansAll(t *testing.T) {
	s := auth.NewStore()
	_, tok, err := s.Create(auth.CreateOptions{})
	require.NoError(t, err)

	assert.True(t, tok.HasPaneAccess("any-pane"))
	assert.True(t, tok.HasPaneAccess("another-pane"))
}

func TestToken_HasPaneAccess_ScopedToList(t *testing.T) {
	s := auth.NewStore()
	_, tok, err := s.Create(auth.CreateOptions{
		Panes: []string{"build-log", "error-log"},
	})
	require.NoError(t, err)

	assert.True(t, tok.HasPaneAccess("build-log"))
	assert.True(t, tok.HasPaneAccess("error-log"))
	assert.False(t, tok.HasPaneAccess("other-pane"))
}

func TestCreate_UniqueTokens(t *testing.T) {
	s := auth.NewStore()
	seen := make(map[string]bool)

	for range 20 {
		plain, _, err := s.Create(auth.CreateOptions{})
		require.NoError(t, err)
		assert.False(t, seen[plain], "duplicate token generated")
		seen[plain] = true
	}
}

func TestStore_Concurrent(t *testing.T) {
	s := auth.NewStore()
	done := make(chan struct{})
	plains := make(chan string, 50)

	for range 25 {
		go func() {
			plain, _, _ := s.Create(auth.CreateOptions{})
			plains <- plain
			done <- struct{}{}
		}()
	}

	for range 25 {
		<-done
	}

	close(plains)

	for plain := range plains {
		go func(p string) {
			_, _ = s.Verify(p)
			done <- struct{}{}
		}(plain)
	}

	for range 25 {
		<-done
	}
}
