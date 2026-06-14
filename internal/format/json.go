package format

import (
	"encoding/json"
	"strings"
)

// JSONFormatter renders JSON lines as syntax-highlighted HTML. Each line is
// parsed independently; malformed lines are emitted as plain escaped text.
// The browser SPA may add interactivity (collapse/expand) via JS.
type JSONFormatter struct{}

func (f *JSONFormatter) Name() string { return NameJSON }

func (f *JSONFormatter) Format(lines []string) (string, error) {
	var b strings.Builder

	b.WriteString("<div class=\"rdw-json\">")

	for _, raw := range lines {
		raw = strings.TrimRight(raw, "\r")
		if raw == "" {
			b.WriteString("<div class=\"rdw-json-blank\"></div>")
			continue
		}

		// Attempt pretty-print + highlight.
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			// Not valid JSON — emit as plain text.
			b.WriteString("<pre class=\"rdw-json-raw\">")
			b.WriteString(escapeHTML(raw))
			b.WriteString("</pre>")
			continue
		}

		pretty, _ := json.MarshalIndent(v, "", "  ")
		b.WriteString("<pre class=\"rdw-json-block\">")
		b.WriteString(highlightJSON(string(pretty)))
		b.WriteString("</pre>")
	}

	b.WriteString("</div>")

	return b.String(), nil
}

// highlightJSON applies simple token-level CSS classes to a pretty-printed
// JSON string. The browser uses these classes for colour via CSS variables.
func highlightJSON(src string) string {
	var b strings.Builder

	i := 0
	n := len(src)

	for i < n {
		c := src[i]
		switch {
		case c == '"':
			// Read string token.
			j := i + 1
			for j < n {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '"' {
					j++
					break
				}
				j++
			}
			token := src[i:j]
			// Determine if this is a key (next non-space char is ':').
			k := j
			for k < n && (src[k] == ' ' || src[k] == '\t') {
				k++
			}
			if k < n && src[k] == ':' {
				b.WriteString("<span class=\"rdw-json-key\">")
				b.WriteString(escapeHTML(token))
				b.WriteString("</span>")
			} else {
				b.WriteString("<span class=\"rdw-json-string\">")
				b.WriteString(escapeHTML(token))
				b.WriteString("</span>")
			}
			i = j

		case c == 't' || c == 'f' || c == 'n':
			// true / false / null
			for _, kw := range []string{"true", "false", "null"} {
				if strings.HasPrefix(src[i:], kw) {
					b.WriteString("<span class=\"rdw-json-literal\">")
					b.WriteString(kw)
					b.WriteString("</span>")
					i += len(kw)
					goto nextChar
				}
			}
			b.WriteByte(c)
			i++

		case c >= '0' && c <= '9' || c == '-':
			// Number
			j := i + 1
			for j < n && (src[j] >= '0' && src[j] <= '9' || src[j] == '.' || src[j] == 'e' || src[j] == 'E' || src[j] == '+' || src[j] == '-') {
				j++
			}
			b.WriteString("<span class=\"rdw-json-number\">")
			b.WriteString(escapeHTML(src[i:j]))
			b.WriteString("</span>")
			i = j

		case c == '{' || c == '}' || c == '[' || c == ']' || c == ':' || c == ',':
			b.WriteString("<span class=\"rdw-json-punct\">")
			b.WriteByte(c)
			b.WriteString("</span>")
			i++

		case c == '\n':
			b.WriteByte('\n')
			i++

		default:
			b.WriteByte(c)
			i++
		}
	nextChar:
	}

	return b.String()
}
