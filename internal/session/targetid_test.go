package session_test

import (
	"strings"
	"testing"

	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTargetID_Valid(t *testing.T) {
	cases := []string{
		"build-log",
		"api_status",
		"pane1",
		"A",
		"My Pane",
		"x y-z_1",
		strings.Repeat("a", 64),
	}

	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			id, err := session.ParseTargetID(tc)
			require.NoError(t, err)
			assert.Equal(t, tc, id.String())
		})
	}
}

func TestParseTargetID_Invalid(t *testing.T) {
	cases := []struct {
		input string
		desc  string
	}{
		{"", "empty string"},
		{strings.Repeat("a", 65), "too long"},
		{"-bad", "starts with dash"},
		{" bad", "starts with space"},
		{"bad/path", "contains slash"},
		{"bad\ttab", "contains tab"},
		{"bad\x00null", "contains null byte"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := session.ParseTargetID(tc.input)
			assert.Error(t, err, "expected error for input %q", tc.input)
		})
	}
}

func TestTargetID_String(t *testing.T) {
	id, err := session.ParseTargetID("my-pane")
	require.NoError(t, err)
	assert.Equal(t, "my-pane", id.String())
}
