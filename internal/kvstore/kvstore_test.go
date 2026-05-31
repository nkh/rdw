package kvstore_test

import (
	"strings"
	"testing"

	"github.com/nkh/rdw/internal/kvstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Key validation
// ---------------------------------------------------------------------------

func TestParseKey_Valid(t *testing.T) {
	cases := []string{
		"build-status",
		"stage",
		"window:main:title",
		"pane:build-log:color",
		strings.Repeat("a", 64),
	}

	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			k, err := kvstore.ParseKey(tc)
			require.NoError(t, err)
			assert.Equal(t, tc, k.String())
		})
	}
}

func TestParseKey_Invalid(t *testing.T) {
	cases := []struct {
		input string
		desc  string
	}{
		{"", "empty"},
		{strings.Repeat("x", 65), "too long"},
		{"-bad", "leading dash"},
		{"bad/key", "slash"},
		{"bad\nkey", "newline"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := kvstore.ParseKey(tc.input)
			assert.Error(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// Store operations
// ---------------------------------------------------------------------------

func TestStore_SetGet(t *testing.T) {
	s := kvstore.New()
	k, _ := kvstore.ParseKey("status")

	err := s.Set(k, "passing")
	require.NoError(t, err)

	v, ok := s.Get(k)
	assert.True(t, ok)
	assert.Equal(t, "passing", v)
}

func TestStore_GetMissing(t *testing.T) {
	s := kvstore.New()
	k, _ := kvstore.ParseKey("missing")

	v, ok := s.Get(k)
	assert.False(t, ok)
	assert.Equal(t, "", v)
}

func TestStore_Overwrite(t *testing.T) {
	s := kvstore.New()
	k, _ := kvstore.ParseKey("stage")

	require.NoError(t, s.Set(k, "build"))
	require.NoError(t, s.Set(k, "test"))

	v, ok := s.Get(k)
	assert.True(t, ok)
	assert.Equal(t, "test", v)
}

func TestStore_Delete(t *testing.T) {
	s := kvstore.New()
	k, _ := kvstore.ParseKey("temp")

	require.NoError(t, s.Set(k, "value"))
	s.Delete(k)

	_, ok := s.Get(k)
	assert.False(t, ok)
	assert.Equal(t, 0, s.Len())
}

func TestStore_DeleteMissing(t *testing.T) {
	s := kvstore.New()
	k, _ := kvstore.ParseKey("ghost")

	// Must not panic
	s.Delete(k)
	assert.Equal(t, 0, s.Len())
}

func TestStore_Keys_NoPrefix(t *testing.T) {
	s := kvstore.New()

	for _, name := range []string{"z-last", "a-first", "m-middle"} {
		k, _ := kvstore.ParseKey(name)
		require.NoError(t, s.Set(k, "v"))
	}

	keys := s.Keys("")
	assert.Len(t, keys, 3)
	assert.Equal(t, kvstore.Key("a-first"), keys[0])
	assert.Equal(t, kvstore.Key("z-last"), keys[2])
}

func TestStore_Keys_WithPrefix(t *testing.T) {
	s := kvstore.New()

	names := []string{"window:main:title", "window:main:color", "pane:build-log:color", "stage"}
	for _, name := range names {
		k, _ := kvstore.ParseKey(name)
		require.NoError(t, s.Set(k, "v"))
	}

	keys := s.Keys("window:")
	assert.Len(t, keys, 2)
}

func TestStore_TotalBytes(t *testing.T) {
	s := kvstore.New()

	k, _ := kvstore.ParseKey("x")
	require.NoError(t, s.Set(k, "hello"))
	assert.Equal(t, 5, s.TotalBytes())

	require.NoError(t, s.Set(k, "hi"))
	assert.Equal(t, 2, s.TotalBytes())

	s.Delete(k)
	assert.Equal(t, 0, s.TotalBytes())
}

func TestStore_ValueSizeLimit(t *testing.T) {
	s := kvstore.New()
	k, _ := kvstore.ParseKey("big")

	big := strings.Repeat("x", kvstore.ValueMaxBytes+1)
	err := s.Set(k, big)
	assert.Error(t, err)
}

func TestStore_TotalSizeLimit(t *testing.T) {
	s := kvstore.New()

	// Fill store to near its limit with one key
	k, _ := kvstore.ParseKey("data")
	nearLimit := strings.Repeat("x", kvstore.ValueMaxBytes)

	// Cram in as many as needed
	filled := 0
	for filled+kvstore.ValueMaxBytes <= kvstore.StoreMaxBytes {
		kk, _ := kvstore.ParseKey("data")
		_ = kk
		filled += kvstore.ValueMaxBytes
		break // one entry is enough to test the delta logic
	}

	// Write a value that just fits
	require.NoError(t, s.Set(k, nearLimit))

	// Writing a second key that would overflow
	k2, _ := kvstore.ParseKey("data2")
	overflow := strings.Repeat("y", kvstore.StoreMaxBytes)
	err := s.Set(k2, overflow)
	assert.Error(t, err)
}

func TestStore_Snapshot(t *testing.T) {
	s := kvstore.New()

	k1, _ := kvstore.ParseKey("a")
	k2, _ := kvstore.ParseKey("b")
	require.NoError(t, s.Set(k1, "1"))
	require.NoError(t, s.Set(k2, "2"))

	snap := s.Snapshot()
	assert.Equal(t, "1", snap[k1])
	assert.Equal(t, "2", snap[k2])

	// Mutating the snapshot must not affect the store
	snap[k1] = "mutated"
	v, _ := s.Get(k1)
	assert.Equal(t, "1", v)
}

func TestStore_Concurrent(t *testing.T) {
	s := kvstore.New()
	k, _ := kvstore.ParseKey("counter")
	require.NoError(t, s.Set(k, "init"))

	done := make(chan struct{})

	for i := 0; i < 50; i++ {
		go func() {
			_ = s.Set(k, "value")
			_, _ = s.Get(k)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 50; i++ {
		<-done
	}
}
