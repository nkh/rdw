package session_test

import (
	"testing"

	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookmark_AddAndGet(t *testing.T) {
	s := session.NewBookmarkStore()
	require.NoError(t, s.Add("start", 0))

	b, ok := s.Get("start")
	assert.True(t, ok)
	assert.Equal(t, "start", b.Name)
	assert.Equal(t, 0, b.LineIndex)
	assert.False(t, b.CreatedAt.IsZero())
}

func TestBookmark_Replace(t *testing.T) {
	s := session.NewBookmarkStore()
	require.NoError(t, s.Add("mark", 10))
	require.NoError(t, s.Add("mark", 20))

	b, _ := s.Get("mark")
	assert.Equal(t, 20, b.LineIndex)
}

func TestBookmark_Remove(t *testing.T) {
	s := session.NewBookmarkStore()
	require.NoError(t, s.Add("x", 5))
	require.NoError(t, s.Remove("x"))

	_, ok := s.Get("x")
	assert.False(t, ok)
}

func TestBookmark_RemoveMissing(t *testing.T) {
	s := session.NewBookmarkStore()
	assert.Error(t, s.Remove("nope"))
}

func TestBookmark_EmptyName(t *testing.T) {
	s := session.NewBookmarkStore()
	assert.Error(t, s.Add("", 0))
}

func TestBookmark_NegativeIndex(t *testing.T) {
	s := session.NewBookmarkStore()
	assert.Error(t, s.Add("bad", -1))
}

func TestBookmark_AllSorted(t *testing.T) {
	s := session.NewBookmarkStore()
	require.NoError(t, s.Add("c", 30))
	require.NoError(t, s.Add("a", 10))
	require.NoError(t, s.Add("b", 20))

	all := s.All()
	assert.Len(t, all, 3)
	assert.Equal(t, 10, all[0].LineIndex)
	assert.Equal(t, 20, all[1].LineIndex)
	assert.Equal(t, 30, all[2].LineIndex)
}

func TestBookmark_Len(t *testing.T) {
	s := session.NewBookmarkStore()
	assert.Equal(t, 0, s.Len())
	_ = s.Add("a", 1)
	assert.Equal(t, 1, s.Len())
}
