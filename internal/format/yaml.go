package format

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLFormatter renders YAML lines as syntax-highlighted HTML.
// Lines are accumulated until a document boundary (--- or ...) or end of input,
// then parsed and re-emitted with CSS classes.
type YAMLFormatter struct{}

func (f *YAMLFormatter) Name() string { return NameYAML }

func (f *YAMLFormatter) Format(lines []string) (string, error) {
	var b strings.Builder

	b.WriteString("<div class=\"rdw-yaml\">")

	// Split input into YAML documents separated by "---".
	docs := splitYAMLDocs(lines)

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var v any
		if err := yaml.Unmarshal([]byte(doc), &v); err != nil {
			b.WriteString("<pre class=\"rdw-yaml-raw\">")
			b.WriteString(escapeHTML(doc))
			b.WriteString("</pre>")
			continue
		}

		// Re-marshal for canonical output.
		out, err := yaml.Marshal(v)
		if err != nil {
			b.WriteString("<pre class=\"rdw-yaml-raw\">")
			b.WriteString(escapeHTML(doc))
			b.WriteString("</pre>")
			continue
		}

		b.WriteString("<pre class=\"rdw-yaml-block\">")
		b.WriteString(highlightYAML(string(out)))
		b.WriteString("</pre>")
	}

	b.WriteString("</div>")

	return b.String(), nil
}

func splitYAMLDocs(lines []string) []string {
	var docs []string
	var cur strings.Builder

	for _, l := range lines {
		if l == "---" || l == "..." {
			docs = append(docs, cur.String())
			cur.Reset()
		} else {
			cur.WriteString(l)
			cur.WriteByte('\n')
		}
	}

	if cur.Len() > 0 {
		docs = append(docs, cur.String())
	}

	return docs
}

// highlightYAML applies CSS classes to key/value tokens in canonical YAML output.
func highlightYAML(src string) string {
	var b strings.Builder

	for _, line := range strings.Split(src, "\n") {
		if line == "" {
			b.WriteByte('\n')
			continue
		}

		// Detect leading indent + key: value pattern.
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]

		if idx := strings.Index(trimmed, ": "); idx > 0 {
			key := trimmed[:idx]
			val := trimmed[idx+2:]
			b.WriteString(escapeHTML(indent))
			b.WriteString("<span class=\"rdw-yaml-key\">")
			b.WriteString(escapeHTML(key))
			b.WriteString("</span>")
			b.WriteString("<span class=\"rdw-yaml-punct\">: </span>")
			b.WriteString(colorYAMLValue(val))
		} else if strings.HasSuffix(trimmed, ":") {
			b.WriteString(escapeHTML(indent))
			b.WriteString("<span class=\"rdw-yaml-key\">")
			b.WriteString(escapeHTML(strings.TrimSuffix(trimmed, ":")))
			b.WriteString("</span>")
			b.WriteString("<span class=\"rdw-yaml-punct\">:</span>")
		} else if strings.HasPrefix(trimmed, "- ") {
			b.WriteString(escapeHTML(indent))
			b.WriteString("<span class=\"rdw-yaml-punct\">- </span>")
			b.WriteString(colorYAMLValue(trimmed[2:]))
		} else {
			b.WriteString(escapeHTML(line))
		}

		b.WriteByte('\n')
	}

	return b.String()
}

func colorYAMLValue(v string) string {
	switch v {
	case "true", "false", "null", "~":
		return "<span class=\"rdw-yaml-literal\">" + escapeHTML(v) + "</span>"
	}

	// Numeric heuristic.
	if len(v) > 0 && (v[0] >= '0' && v[0] <= '9' || v[0] == '-') {
		return "<span class=\"rdw-yaml-number\">" + escapeHTML(v) + "</span>"
	}

	if strings.HasPrefix(v, "\"") || strings.HasPrefix(v, "'") {
		return "<span class=\"rdw-yaml-string\">" + escapeHTML(v) + "</span>"
	}

	return "<span class=\"rdw-yaml-value\">" + escapeHTML(v) + "</span>"
}
