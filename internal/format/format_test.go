package format_test

import (
	"strings"
	"testing"

	"github.com/nkh/rdw/internal/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_KnownFormatters(t *testing.T) {
	for _, name := range []string{"text", "json", "yaml", "markdown", "csv", "image"} {
		f, err := format.Get(name)
		require.NoError(t, err, name)
		assert.Equal(t, name, f.Name())
	}
}

func TestGet_Unknown(t *testing.T) {
	_, err := format.Get("nope")
	assert.Error(t, err)
}

func TestTextFormatter(t *testing.T) {
	f, _ := format.Get("text")
	out, err := f.Format([]string{"hello <world>", "line 2"})
	require.NoError(t, err)
	assert.Contains(t, out, "hello &lt;world&gt;")
	assert.Contains(t, out, "line 2")
	assert.Contains(t, out, "<pre")
}

func TestJSONFormatter_Valid(t *testing.T) {
	f, _ := format.Get("json")
	out, err := f.Format([]string{`{"name":"rdw","count":3,"ok":true}`})
	require.NoError(t, err)
	assert.Contains(t, out, "rdw-json-key")
	assert.Contains(t, out, "rdw-json-string")
	assert.Contains(t, out, "rdw-json-number")
	assert.Contains(t, out, "rdw-json-literal")
}

func TestJSONFormatter_Invalid(t *testing.T) {
	f, _ := format.Get("json")
	out, err := f.Format([]string{"not json"})
	require.NoError(t, err)
	assert.Contains(t, out, "rdw-json-raw")
	assert.Contains(t, out, "not json")
}

func TestJSONFormatter_EmptyLine(t *testing.T) {
	f, _ := format.Get("json")
	out, err := f.Format([]string{""})
	require.NoError(t, err)
	assert.Contains(t, out, "rdw-json-blank")
}

func TestYAMLFormatter_Valid(t *testing.T) {
	f, _ := format.Get("yaml")
	out, err := f.Format([]string{"name: rdw", "version: 1"})
	require.NoError(t, err)
	assert.Contains(t, out, "rdw-yaml-key")
	assert.Contains(t, out, "name")
}

func TestYAMLFormatter_MultiDoc(t *testing.T) {
	f, _ := format.Get("yaml")
	out, err := f.Format([]string{"a: 1", "---", "b: 2"})
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(out, "rdw-yaml-block"))
}

func TestYAMLFormatter_Invalid(t *testing.T) {
	f, _ := format.Get("yaml")
	// YAML is very permissive; force a type error by passing something
	// that parses but triggers a render path — just check no panic.
	out, err := f.Format([]string{"key: [unclosed"})
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

func TestMarkdownFormatter_Heading(t *testing.T) {
	f, _ := format.Get("markdown")
	out, err := f.Format([]string{"# Title", "## Sub"})
	require.NoError(t, err)
	assert.Contains(t, out, "<h1")
	assert.Contains(t, out, "<h2")
}

func TestMarkdownFormatter_Bold(t *testing.T) {
	f, _ := format.Get("markdown")
	out, err := f.Format([]string{"**bold** and *italic*"})
	require.NoError(t, err)
	assert.Contains(t, out, "<strong>bold</strong>")
	assert.Contains(t, out, "<em>italic</em>")
}

func TestMarkdownFormatter_CodeFence(t *testing.T) {
	f, _ := format.Get("markdown")
	out, err := f.Format([]string{"```go", "func main() {}", "```"})
	require.NoError(t, err)
	assert.Contains(t, out, "<code class=\"language-go\">")
	assert.Contains(t, out, "func main()")
}

func TestMarkdownFormatter_List(t *testing.T) {
	f, _ := format.Get("markdown")
	out, err := f.Format([]string{"- item a", "- item b"})
	require.NoError(t, err)
	assert.Contains(t, out, "<ul")
	assert.Contains(t, out, "<li>item a</li>")
}

func TestMarkdownFormatter_OrderedList(t *testing.T) {
	f, _ := format.Get("markdown")
	out, err := f.Format([]string{"1. first", "2. second"})
	require.NoError(t, err)
	assert.Contains(t, out, "<ol")
}

func TestCSVFormatter_Basic(t *testing.T) {
	f, _ := format.Get("csv")
	out, err := f.Format([]string{"name,age,city", "alice,30,london", "bob,25,paris"})
	require.NoError(t, err)
	assert.Contains(t, out, "<th")
	assert.Contains(t, out, "alice")
	assert.Contains(t, out, "paris")
}

func TestCSVFormatter_TSV(t *testing.T) {
	f, _ := format.Get("csv")
	out, err := f.Format([]string{"a\tb\tc", "1\t2\t3"})
	require.NoError(t, err)
	assert.Contains(t, out, "<th")
	assert.Contains(t, out, "1")
}

func TestCSVFormatter_Empty(t *testing.T) {
	f, _ := format.Get("csv")
	out, err := f.Format([]string{})
	require.NoError(t, err)
	assert.Contains(t, out, "rdw-csv")
}

func TestImageFormatter_PNG(t *testing.T) {
	import_b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	f, _ := format.Get("image")
	out, err := f.Format([]string{import_b64})
	require.NoError(t, err)
	assert.Contains(t, out, "image/png")
	assert.Contains(t, out, "<img")
}

func TestImageFormatter_WithPrefix(t *testing.T) {
	import_b64 := "b64:iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	f, _ := format.Get("image")
	out, err := f.Format([]string{import_b64})
	require.NoError(t, err)
	assert.Contains(t, out, "image/png")
}

func TestImageFormatter_Invalid(t *testing.T) {
	f, _ := format.Get("image")
	out, err := f.Format([]string{"not-base64!!!"})
	require.NoError(t, err)
	assert.Contains(t, out, "rdw-image-err")
}

func TestImageFormatter_Empty(t *testing.T) {
	f, _ := format.Get("image")
	out, err := f.Format([]string{})
	require.NoError(t, err)
	assert.Contains(t, out, "rdw-image")
}
