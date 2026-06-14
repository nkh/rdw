package format

import (
	"strings"
)

// MarkdownFormatter converts Markdown lines to HTML using a lightweight
// single-pass renderer. Supports headings, bold, italic, inline code, code
// fences, unordered and ordered lists, blockquotes, and paragraphs.
type MarkdownFormatter struct{}

func (f *MarkdownFormatter) Name() string { return NameMarkdown }

func (f *MarkdownFormatter) Format(lines []string) (string, error) {
	var b strings.Builder

	b.WriteString("<div class=\"rdw-markdown\">")
	b.WriteString(renderMarkdown(lines))
	b.WriteString("</div>")

	return b.String(), nil
}

func renderMarkdown(lines []string) string {
	var b strings.Builder
	i := 0
	n := len(lines)

	for i < n {
		line := lines[i]

		// Code fence.
		if strings.HasPrefix(line, "```") {
			lang := strings.TrimPrefix(line, "```")
			i++
			b.WriteString("<pre class=\"rdw-md-code\"><code")
			if lang != "" {
				b.WriteString(" class=\"language-")
				b.WriteString(escapeHTML(lang))
				b.WriteString("\"")
			}
			b.WriteString(">")
			for i < n && !strings.HasPrefix(lines[i], "```") {
				b.WriteString(escapeHTML(lines[i]))
				b.WriteByte('\n')
				i++
			}
			b.WriteString("</code></pre>")
			i++ // consume closing ```
			continue
		}

		// Headings.
		if strings.HasPrefix(line, "#") {
			level := 0
			for level < len(line) && line[level] == '#' {
				level++
			}
			if level <= 6 && (len(line) == level || line[level] == ' ') {
				text := strings.TrimSpace(line[level:])
				b.WriteString("<h")
				b.WriteByte(byte('0' + level))
				b.WriteString(" class=\"rdw-md-h\">")
				b.WriteString(inlineMarkdown(text))
				b.WriteString("</h")
				b.WriteByte(byte('0' + level))
				b.WriteString(">")
				i++
				continue
			}
		}

		// Horizontal rule.
		if line == "---" || line == "***" || line == "___" {
			b.WriteString("<hr class=\"rdw-md-hr\">")
			i++
			continue
		}

		// Blockquote.
		if strings.HasPrefix(line, "> ") {
			b.WriteString("<blockquote class=\"rdw-md-bq\">")
			for i < n && strings.HasPrefix(lines[i], "> ") {
				b.WriteString("<p>")
				b.WriteString(inlineMarkdown(lines[i][2:]))
				b.WriteString("</p>")
				i++
			}
			b.WriteString("</blockquote>")
			continue
		}

		// Unordered list.
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			b.WriteString("<ul class=\"rdw-md-ul\">")
			for i < n && (strings.HasPrefix(lines[i], "- ") || strings.HasPrefix(lines[i], "* ")) {
				b.WriteString("<li>")
				b.WriteString(inlineMarkdown(lines[i][2:]))
				b.WriteString("</li>")
				i++
			}
			b.WriteString("</ul>")
			continue
		}

		// Ordered list (simplified: starts with digit+dot).
		if len(line) > 2 && line[0] >= '1' && line[0] <= '9' && line[1] == '.' && line[2] == ' ' {
			b.WriteString("<ol class=\"rdw-md-ol\">")
			for i < n && len(lines[i]) > 2 && lines[i][0] >= '0' && lines[i][0] <= '9' && lines[i][1] == '.' {
				rest := strings.SplitN(lines[i], ". ", 2)
				if len(rest) == 2 {
					b.WriteString("<li>")
					b.WriteString(inlineMarkdown(rest[1]))
					b.WriteString("</li>")
				}
				i++
			}
			b.WriteString("</ol>")
			continue
		}

		// Blank line.
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		// Paragraph.
		b.WriteString("<p class=\"rdw-md-p\">")
		for i < n && strings.TrimSpace(lines[i]) != "" &&
			!strings.HasPrefix(lines[i], "#") &&
			!strings.HasPrefix(lines[i], "```") &&
			!strings.HasPrefix(lines[i], "> ") &&
			!strings.HasPrefix(lines[i], "- ") &&
			!strings.HasPrefix(lines[i], "* ") {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(inlineMarkdown(lines[i]))
			i++
		}
		b.WriteString("</p>")
	}

	return b.String()
}

// inlineMarkdown handles bold, italic, inline code, and links.
func inlineMarkdown(s string) string {
	var b strings.Builder
	i := 0
	n := len(s)

	for i < n {
		c := s[i]
		switch {
		case c == '`':
			j := strings.Index(s[i+1:], "`")
			if j >= 0 {
				b.WriteString("<code class=\"rdw-md-ic\">")
				b.WriteString(escapeHTML(s[i+1 : i+1+j]))
				b.WriteString("</code>")
				i = i + 1 + j + 1
			} else {
				b.WriteString(escapeHTML(string(c)))
				i++
			}

		case c == '*' || c == '_':
			delim := string(c)
			// Bold (**) or italic (*).
			if i+1 < n && s[i+1] == c {
				end := strings.Index(s[i+2:], delim+delim)
				if end >= 0 {
					b.WriteString("<strong>")
					b.WriteString(escapeHTML(s[i+2 : i+2+end]))
					b.WriteString("</strong>")
					i = i + 2 + end + 2
				} else {
					b.WriteString(escapeHTML(string(c)))
					i++
				}
			} else {
				end := strings.Index(s[i+1:], delim)
				if end >= 0 {
					b.WriteString("<em>")
					b.WriteString(escapeHTML(s[i+1 : i+1+end]))
					b.WriteString("</em>")
					i = i + 1 + end + 1
				} else {
					b.WriteString(escapeHTML(string(c)))
					i++
				}
			}

		case c == '[':
			// [text](url)
			textEnd := strings.Index(s[i+1:], "]")
			if textEnd >= 0 && i+2+textEnd < n && s[i+1+textEnd+1] == '(' {
				urlStart := i + 1 + textEnd + 2
				urlEnd := strings.Index(s[urlStart:], ")")
				if urlEnd >= 0 {
					text := s[i+1 : i+1+textEnd]
					url := s[urlStart : urlStart+urlEnd]
					b.WriteString("<a href=\"")
					b.WriteString(escapeHTML(url))
					b.WriteString("\" target=\"_blank\">")
					b.WriteString(escapeHTML(text))
					b.WriteString("</a>")
					i = urlStart + urlEnd + 1
					continue
				}
			}
			b.WriteString(escapeHTML(string(c)))
			i++

		default:
			b.WriteString(escapeHTML(string(c)))
			i++
		}
	}

	return b.String()
}
