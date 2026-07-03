package pipe_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/nkh/rdw/internal/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHybridReader_PlainLines(t *testing.T) {
	r := pipe.NewHybridReaderExported(strings.NewReader("hello\nworld\n"))

	require.True(t, r.Next())
	assert.Equal(t, "hello", r.Text())

	require.True(t, r.Next())
	assert.Equal(t, "world", r.Text())

	assert.False(t, r.Next())
	assert.NoError(t, r.Err())
}

func TestHybridReader_ImageSentinel(t *testing.T) {
	// Simulates: image:\n<binary data lines>\nimage:end
	input := "before\nimage:\nABC\nDEF\nimage:end\nafter\n"
	r := pipe.NewHybridReaderExported(strings.NewReader(input))

	require.True(t, r.Next())
	assert.Equal(t, "before", r.Text())

	require.True(t, r.Next())
	line := r.Text()
	assert.True(t, strings.HasPrefix(line, "b64:"), "expected b64: prefix, got: %q", line)

	// Decode and verify the accumulated binary content.
	encoded := strings.TrimPrefix(line, "b64:")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, "ABC\nDEF\n", string(decoded))

	require.True(t, r.Next())
	assert.Equal(t, "after", r.Text())

	assert.False(t, r.Next())
}

func TestHybridReader_SVGSentinel(t *testing.T) {
	svgContent := "<svg><rect/></svg>\n"
	input := "svg:\n" + svgContent + "svg:end\n"
	r := pipe.NewHybridReaderExported(strings.NewReader(input))

	require.True(t, r.Next())
	line := r.Text()
	assert.True(t, strings.HasPrefix(line, "svg-data:"), "expected svg-data: prefix, got: %q", line)

	encoded := strings.TrimPrefix(line, "svg-data:")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, svgContent, string(decoded))

	assert.False(t, r.Next())
}

func TestHybridReader_EmptyImageBlock(t *testing.T) {
	input := "image:\nimage:end\n"
	r := pipe.NewHybridReaderExported(strings.NewReader(input))

	require.True(t, r.Next())
	line := r.Text()
	assert.True(t, strings.HasPrefix(line, "b64:"))
}

func TestHybridReader_MultipleBlocks(t *testing.T) {
	input := "a\nimage:\nDATA\nimage:end\nb\nsvg:\n<svg/>\nsvg:end\nc\n"
	r := pipe.NewHybridReaderExported(strings.NewReader(input))

	lines := []string{}
	for r.Next() {
		lines = append(lines, r.Text())
	}
	require.NoError(t, r.Err())

	require.Len(t, lines, 5)
	assert.Equal(t, "a", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "b64:"))
	assert.Equal(t, "b", lines[2])
	assert.True(t, strings.HasPrefix(lines[3], "svg-data:"))
	assert.Equal(t, "c", lines[4])
}
