package highlight_test

import (
	"testing"

	"github.com/nkh/rdw/internal/highlight"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdd_Valid(t *testing.T) {
	s := highlight.New()
	err := s.Add(highlight.Profile{
		Name: "errors",
		Rules: []highlight.Rule{
			{Pattern: `ERROR`, Class: "hl-error"},
			{Pattern: `WARN\w+`, Class: "hl-warn"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, s.Len())
}

func TestAdd_EmptyName(t *testing.T) {
	s := highlight.New()
	assert.Error(t, s.Add(highlight.Profile{Name: ""}))
}

func TestAdd_BadPattern(t *testing.T) {
	s := highlight.New()
	err := s.Add(highlight.Profile{
		Name:  "bad",
		Rules: []highlight.Rule{{Pattern: `[invalid`, Class: "x"}},
	})
	assert.Error(t, err)
}

func TestAdd_EmptyClass(t *testing.T) {
	s := highlight.New()
	err := s.Add(highlight.Profile{
		Name:  "p",
		Rules: []highlight.Rule{{Pattern: `ok`, Class: ""}},
	})
	assert.Error(t, err)
}

func TestAdd_Replaces(t *testing.T) {
	s := highlight.New()
	require.NoError(t, s.Add(highlight.Profile{Name: "p", Rules: []highlight.Rule{{Pattern: `a`, Class: "x"}}}))
	require.NoError(t, s.Add(highlight.Profile{Name: "p", Rules: []highlight.Rule{{Pattern: `b`, Class: "y"}}}))

	p, _ := s.Get("p")
	assert.Equal(t, "b", p.Rules[0].Pattern)
}

func TestRemove_Existing(t *testing.T) {
	s := highlight.New()
	require.NoError(t, s.Add(highlight.Profile{Name: "x", Rules: []highlight.Rule{{Pattern: `x`, Class: "c"}}}))
	require.NoError(t, s.Remove("x"))
	assert.Equal(t, 0, s.Len())
}

func TestRemove_Missing(t *testing.T) {
	s := highlight.New()
	assert.Error(t, s.Remove("nope"))
}

func TestGet_Missing(t *testing.T) {
	s := highlight.New()
	_, ok := s.Get("nope")
	assert.False(t, ok)
}

func TestAll(t *testing.T) {
	s := highlight.New()
	require.NoError(t, s.Add(highlight.Profile{Name: "a", Rules: []highlight.Rule{{Pattern: `a`, Class: "ca"}}}))
	require.NoError(t, s.Add(highlight.Profile{Name: "b", Rules: []highlight.Rule{{Pattern: `b`, Class: "cb"}}}))

	all := s.All()
	assert.Len(t, all, 2)
}
