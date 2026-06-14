// Package format provides line-oriented content formatters for rdw panes.
// Each formatter receives raw lines and returns HTML fragments for browser rendering.
package format

import (
	"fmt"
	"strings"
)

// Formatter transforms a slice of raw lines into an HTML string for browser rendering.
type Formatter interface {
	Format(lines []string) (string, error)
	Name() string
}

// Known formatter names.
const (
	NameText     = "text"
	NameJSON     = "json"
	NameYAML     = "yaml"
	NameMarkdown = "markdown"
	NameCSV      = "csv"
	NameImage    = "image"
)

var registry = map[string]Formatter{
	NameText:     &TextFormatter{},
	NameJSON:     &JSONFormatter{},
	NameYAML:     &YAMLFormatter{},
	NameMarkdown: &MarkdownFormatter{},
	NameCSV:      &CSVFormatter{},
	NameImage:    &ImageFormatter{},
}

// Get returns the formatter for the given name, or an error if unknown.
func Get(name string) (Formatter, error) {
	f, ok := registry[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("unknown formatter %q; valid names: text, json, yaml, markdown, csv, image", name)
	}

	return f, nil
}

// Names returns all registered formatter names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}

	return names
}

// escapeHTML escapes the five special HTML characters.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&#34;")
	s = strings.ReplaceAll(s, "'", "&#39;")

	return s
}
