package control_test

import (
	"testing"

	"github.com/nkh/rdw/internal/control"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

func TestParse_KnownPrefixes(t *testing.T) {
	cases := []struct {
		line    string
		kind    control.Kind
		payload string
	}{
		{"v:raw content", control.KindVerbatim, "raw content"},
		{"q:", control.KindQuit, ""},
		{"s:", control.KindSemaphore, ""},
		{"c:", control.KindClear, ""},
		{"t:", control.KindTimestamp, ""},
		{"f:my_formatter", control.KindFormatter, "my_formatter"},
		{"r:localhost:9000:[1234]", control.KindRelay, "localhost:9000:[1234]"},
		{"=:key=value;key2=value2", control.KindKV, "key=value;key2=value2"},
	}

	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			seq, ok := control.Parse(tc.line)
			require.True(t, ok)
			assert.Equal(t, tc.kind, seq.Kind)
			assert.Equal(t, tc.payload, seq.Payload)
		})
	}
}

func TestParse_UnknownPrefix(t *testing.T) {
	cases := []string{
		"x:something",
		"z:data",
		"build started",
		"",
		"a",
		"::colon",
	}

	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			_, ok := control.Parse(line)
			assert.False(t, ok, "expected no parse for %q", line)
		})
	}
}

func TestParse_VerbatimIsNotContent(t *testing.T) {
	seq, ok := control.Parse("v:=:key=value")
	require.True(t, ok)
	assert.True(t, seq.IsVerbatim())
	assert.Equal(t, "=:key=value", seq.Payload)
}

// ---------------------------------------------------------------------------
// KVPairs
// ---------------------------------------------------------------------------

func TestKVPairs_Single(t *testing.T) {
	seq, _ := control.Parse("=:stage=build")
	pairs, err := seq.KVPairs()
	require.NoError(t, err)
	require.Len(t, pairs, 1)
	assert.Equal(t, "stage", pairs[0][0])
	assert.Equal(t, "build", pairs[0][1])
}

func TestKVPairs_Multiple(t *testing.T) {
	seq, _ := control.Parse("=:stage=build;status=passing;duration=42s")
	pairs, err := seq.KVPairs()
	require.NoError(t, err)
	require.Len(t, pairs, 3)
	assert.Equal(t, [2]string{"stage", "build"}, pairs[0])
	assert.Equal(t, [2]string{"status", "passing"}, pairs[1])
	assert.Equal(t, [2]string{"duration", "42s"}, pairs[2])
}

func TestKVPairs_EmptyPayload(t *testing.T) {
	seq, _ := control.Parse("=:")
	pairs, err := seq.KVPairs()
	require.NoError(t, err)
	assert.Empty(t, pairs)
}

func TestKVPairs_MalformedPair(t *testing.T) {
	seq, _ := control.Parse("=:noequals")
	_, err := seq.KVPairs()
	assert.Error(t, err)
}

func TestKVPairs_TrailingSemicolon(t *testing.T) {
	seq, _ := control.Parse("=:key=val;")
	pairs, err := seq.KVPairs()
	require.NoError(t, err)
	assert.Len(t, pairs, 1)
}

func TestKVPairs_WrongKind(t *testing.T) {
	seq, _ := control.Parse("f:my_formatter")
	_, err := seq.KVPairs()
	assert.Error(t, err)
}

func TestKVPairs_ValueContainsEquals(t *testing.T) {
	seq, _ := control.Parse("=:url=https://example.com/path?a=1")
	pairs, err := seq.KVPairs()
	require.NoError(t, err)
	require.Len(t, pairs, 1)
	assert.Equal(t, "url", pairs[0][0])
	assert.Equal(t, "https://example.com/path?a=1", pairs[0][1])
}
