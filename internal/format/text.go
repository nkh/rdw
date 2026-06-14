package format

import "strings"

// TextFormatter wraps each line in a <pre> span; ANSI parsing is handled
// client-side in the browser SPA. Server-side we just HTML-escape.
type TextFormatter struct{}

func (f *TextFormatter) Name() string { return NameText }

func (f *TextFormatter) Format(lines []string) (string, error) {
	var b strings.Builder

	b.WriteString("<pre class=\"rdw-text\">")

	for _, l := range lines {
		b.WriteString(escapeHTML(l))
		b.WriteByte('\n')
	}

	b.WriteString("</pre>")

	return b.String(), nil
}
