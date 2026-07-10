// build_book.go — minimal mdbook-style static site generator.
// Usage: go run build_book.go
// Reads book/ directory with SUMMARY.md and source .md files.
// Outputs static HTML to book/html/.
package main

import (
	"bufio"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const css = `
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font: 15px/1.65 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
       background: #1a1a2e; color: #c0c0d0; display: flex; min-height: 100vh; }
#sidebar { width: 260px; min-height: 100vh; background: #12121f; padding: 20px 0;
           position: fixed; top: 0; left: 0; overflow-y: auto; border-right: 1px solid #2a2a3e; }
#sidebar h1 { font-size: 14px; font-weight: 700; color: #fff; padding: 0 20px 16px;
              border-bottom: 1px solid #2a2a3e; letter-spacing: .5px; }
#sidebar ul { list-style: none; padding: 8px 0; }
#sidebar li a { display: block; padding: 5px 20px; color: #9090b0; text-decoration: none;
                font-size: 13px; border-left: 3px solid transparent; }
#sidebar li a:hover, #sidebar li a.active { color: #e0e0f0; background: #1e1e32;
                                             border-left-color: #6060c0; }
#sidebar li.section { padding: 12px 20px 4px; font-size: 11px; font-weight: 700;
                       color: #4a4a6a; text-transform: uppercase; letter-spacing: 1px; }
#content { margin-left: 260px; padding: 40px 60px; max-width: 900px; width: 100%; }
h1 { font-size: 28px; color: #e0e0ff; margin-bottom: 24px; padding-bottom: 12px;
     border-bottom: 1px solid #2a2a3e; }
h2 { font-size: 20px; color: #c0c0e0; margin: 32px 0 12px; }
h3 { font-size: 16px; color: #a0a0d0; margin: 24px 0 8px; }
h4 { font-size: 14px; color: #9090c0; margin: 16px 0 6px; }
p  { margin-bottom: 14px; }
a  { color: #8080ff; text-decoration: none; }
a:hover { text-decoration: underline; }
code { background: #1e1e32; color: #c0e0c0; padding: 1px 5px; border-radius: 3px;
       font: 13px "JetBrains Mono", "Fira Code", monospace; }
pre  { background: #0e0e1c; border: 1px solid #2a2a3e; border-radius: 6px;
       padding: 16px; overflow-x: auto; margin-bottom: 16px; }
pre code { background: none; padding: 0; color: #b0d0b0; font-size: 13px; }
table { border-collapse: collapse; width: 100%; margin-bottom: 16px; }
th { background: #1e1e32; color: #a0a0d0; text-align: left; padding: 8px 12px;
     border: 1px solid #2a2a3e; font-size: 13px; }
td { padding: 7px 12px; border: 1px solid #2a2a3e; font-size: 13px; }
tr:nth-child(even) td { background: #16162a; }
ul, ol { margin: 0 0 14px 24px; }
li { margin-bottom: 4px; }
blockquote { border-left: 3px solid #4040a0; padding: 8px 16px; color: #8080a0;
             margin-bottom: 14px; background: #16162a; }
hr { border: none; border-top: 1px solid #2a2a3e; margin: 24px 0; }
#nav-bar { display: flex; justify-content: space-between; margin-top: 40px;
           padding-top: 16px; border-top: 1px solid #2a2a3e; }
#nav-bar a { color: #6060c0; font-size: 13px; }
`

type entry struct {
	title  string
	file   string // source .md path relative to src/
	level  int
	isSection bool
}

func main() {
	bookDir := "book"
	srcDir  := filepath.Join(bookDir, "src")
	outDir  := filepath.Join(bookDir, "html")

	if err := os.MkdirAll(outDir, 0o755) ; err != nil {
		die(err)
	}

	// Read book title from book.toml.
	bookTitle := "rdw"
	if data, err := os.ReadFile(filepath.Join(bookDir, "book.toml")) ; err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "title") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					bookTitle = strings.Trim(strings.TrimSpace(parts[1]), `"`)
				}
			}
		}
	}

	// Parse SUMMARY.md.
	entries := parseSummary(filepath.Join(srcDir, "SUMMARY.md"))

	// Render each page.
	for i, e := range entries {
		if e.isSection || e.file == "" {
			continue
		}

		mdPath := filepath.Join(srcDir, e.file)
		mdData, err := os.ReadFile(mdPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read %s: %v\n", mdPath, err)
			continue
		}

		bodyHTML := mdToHTML(string(mdData))
		outName  := strings.TrimSuffix(e.file, ".md") + ".html"
		outPath  := filepath.Join(outDir, outName)

		if err := os.MkdirAll(filepath.Dir(outPath), 0o755) ; err != nil {
			die(err)
		}

		// Previous / next navigation.
		var prev, next *entry
		for j := i - 1 ; j >= 0 ; j-- {
			if !entries[j].isSection && entries[j].file != "" {
				prev = &entries[j]
				break
			}
		}
		for j := i + 1 ; j < len(entries) ; j++ {
			if !entries[j].isSection && entries[j].file != "" {
				next = &entries[j]
				break
			}
		}

		sidebarHTML := buildSidebar(entries, e.file, bookTitle)
		navHTML     := buildNav(prev, next, e.file)

		page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s — %s</title>
<style>%s</style>
</head>
<body>
<nav id="sidebar">%s</nav>
<main id="content">
%s
%s
</main>
</body>
</html>`, html.EscapeString(e.title), html.EscapeString(bookTitle), css,
			sidebarHTML, bodyHTML, navHTML)

		if err := os.WriteFile(outPath, []byte(page), 0o644) ; err != nil {
			die(err)
		}

		fmt.Printf("  wrote %s\n", outPath)
	}

	// Write index.html redirecting to first page.
	first := ""
	for _, e := range entries {
		if !e.isSection && e.file != "" {
			first = strings.TrimSuffix(e.file, ".md") + ".html"
			break
		}
	}
	if first != "" {
		idx := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8">
<meta http-equiv="refresh" content="0;url=%s">
</head><body><a href="%s">Continue</a></body></html>`, first, first)
		_ = os.WriteFile(filepath.Join(outDir, "index.html"), []byte(idx), 0o644)
	}

	fmt.Printf("\nBook built → %s/\n", outDir)
}

func parseSummary(path string) []entry {
	f, err := os.Open(path)
	if err != nil {
		die(err)
	}
	defer f.Close()

	var entries []entry
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		// Section headings (##).
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			text := strings.TrimLeft(line, "# ")
			entries = append(entries, entry{title: text, isSection: true})
			continue
		}

		m := linkRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		indent := 0
		for _, c := range line {
			if c == ' ' || c == '\t' {
				indent++
			} else {
				break
			}
		}

		entries = append(entries, entry{
			title: m[1],
			file:  m[2],
			level: indent / 2,
		})
	}

	return entries
}

func buildSidebar(entries []entry, activeFile, bookTitle string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<h1>%s</h1><ul>", html.EscapeString(bookTitle)))

	for _, e := range entries {
		if e.isSection {
			b.WriteString(fmt.Sprintf(`<li class="section">%s</li>`, html.EscapeString(e.title)))
			continue
		}
		if e.file == "" {
			continue
		}

		href := strings.TrimSuffix(e.file, ".md") + ".html"
		rel  := relPath(activeFile, href)
		cls  := ""
		if e.file == activeFile {
			cls = ` class="active"`
		}
		pad := strings.Repeat("&nbsp;&nbsp;", e.level)
		b.WriteString(fmt.Sprintf(`<li><a href="%s"%s>%s%s</a></li>`,
			rel, cls, pad, html.EscapeString(e.title)))
	}

	b.WriteString("</ul>")
	return b.String()
}

func buildNav(prev, next *entry, current string) string {
	var b strings.Builder
	b.WriteString(`<div id="nav-bar">`)

	if prev != nil {
		href := relPath(current, strings.TrimSuffix(prev.file, ".md")+".html")
		b.WriteString(fmt.Sprintf(`<a href="%s">← %s</a>`, href, html.EscapeString(prev.title)))
	} else {
		b.WriteString("<span></span>")
	}

	if next != nil {
		href := relPath(current, strings.TrimSuffix(next.file, ".md")+".html")
		b.WriteString(fmt.Sprintf(`<a href="%s">%s →</a>`, href, html.EscapeString(next.title)))
	} else {
		b.WriteString("<span></span>")
	}

	b.WriteString("</div>")
	return b.String()
}

// relPath returns the relative URL from current page to target.
func relPath(from, to string) string {
	fromDir := filepath.Dir(from)
	rel, err := filepath.Rel(fromDir, to)
	if err != nil {
		return to
	}
	return filepath.ToSlash(rel)
}

// mdToHTML converts a subset of Markdown to HTML.
func mdToHTML(md string) string {
	var b strings.Builder
	lines := strings.Split(md, "\n")
	i     := 0
	n     := len(lines)

	inCode   := false
	codeLang := ""
	var codeBuf strings.Builder

	inTable := false
	var tableRows []string

	inList  := false
	listOl  := false
	var listBuf strings.Builder

	flush := func() {
		if inList {
			tag := "ul"
			if listOl {
				tag = "ol"
			}
			b.WriteString("<" + tag + ">" + listBuf.String() + "</" + tag + ">\n")
			listBuf.Reset()
			inList = false
		}
		if inTable {
			b.WriteString("<table>\n")
			for j, row := range tableRows {
				cells := strings.Split(strings.Trim(row, "|"), "|")
				tag := "td"
				if j == 0 {
					tag = "th"
				}
				if j == 1 {
					continue // separator row
				}
				b.WriteString("<tr>")
				for _, c := range cells {
					b.WriteString("<" + tag + ">" + inlineMd(strings.TrimSpace(c)) + "</" + tag + ">")
				}
				b.WriteString("</tr>\n")
			}
			b.WriteString("</table>\n")
			tableRows = nil
			inTable = false
		}
	}

	for i < n {
		line := lines[i]

		// Code fence.
		if strings.HasPrefix(line, "```") {
			if inCode {
				b.WriteString("<pre><code")
				if codeLang != "" {
					b.WriteString(fmt.Sprintf(` class="language-%s"`, html.EscapeString(codeLang)))
				}
				b.WriteString(">")
				b.WriteString(html.EscapeString(codeBuf.String()))
				b.WriteString("</code></pre>\n")
				codeBuf.Reset()
				inCode   = false
				codeLang = ""
			} else {
				flush()
				inCode   = true
				codeLang = strings.TrimPrefix(line, "```")
			}
			i++
			continue
		}

		if inCode {
			codeBuf.WriteString(line + "\n")
			i++
			continue
		}

		// Table row.
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			inTable = true
			tableRows = append(tableRows, line)
			i++
			continue
		} else if inTable {
			flush()
		}

		// Headings.
		if strings.HasPrefix(line, "#") {
			flush()
			level := 0
			for level < len(line) && line[level] == '#' {
				level++
			}
			if level <= 4 {
				tag  := fmt.Sprintf("h%d", level)
				text := strings.TrimSpace(line[level:])
				b.WriteString(fmt.Sprintf("<%s>%s</%s>\n", tag, inlineMd(text), tag))
				i++
				continue
			}
		}

		// HR.
		if line == "---" || line == "***" {
			flush()
			b.WriteString("<hr>\n")
			i++
			continue
		}

		// Blockquote.
		if strings.HasPrefix(line, "> ") {
			flush()
			b.WriteString("<blockquote>")
			for i < n && strings.HasPrefix(lines[i], "> ") {
				b.WriteString("<p>" + inlineMd(lines[i][2:]) + "</p>")
				i++
			}
			b.WriteString("</blockquote>\n")
			continue
		}

		// Ordered list.
		olRe := regexp.MustCompile(`^\d+\. `)
		if olRe.MatchString(line) {
			if !inList || !listOl {
				flush()
				inList  = true
				listOl  = true
			}
			text := olRe.ReplaceAllString(line, "")
			listBuf.WriteString("<li>" + inlineMd(text) + "</li>\n")
			i++
			continue
		}

		// Unordered list.
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			if !inList || listOl {
				flush()
				inList = true
				listOl = false
			}
			listBuf.WriteString("<li>" + inlineMd(line[2:]) + "</li>\n")
			i++
			continue
		}

		// Blank line.
		if strings.TrimSpace(line) == "" {
			flush()
			i++
			continue
		}

		// Paragraph.
		flush()
		var para strings.Builder
		for i < n && strings.TrimSpace(lines[i]) != "" &&
			!strings.HasPrefix(lines[i], "#") &&
			!strings.HasPrefix(lines[i], "```") &&
			!strings.HasPrefix(lines[i], "- ") &&
			!strings.HasPrefix(lines[i], "> ") &&
			lines[i] != "---" {
			if para.Len() > 0 {
				para.WriteString(" ")
			}
			para.WriteString(inlineMd(lines[i]))
			i++
		}
		b.WriteString("<p>" + para.String() + "</p>\n")
	}

	flush()
	return b.String()
}

// inlineMd handles bold, italic, inline code, links.
func inlineMd(s string) string {
	// Inline code first (protect from other processing).
	codeRe := regexp.MustCompile("`([^`]+)`")
	placeholders := map[string]string{}
	idx := 0
	s = codeRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := codeRe.FindStringSubmatch(m)[1]
		key   := fmt.Sprintf("\x00%d\x00", idx)
		idx++
		placeholders[key] = "<code>" + html.EscapeString(inner) + "</code>"
		return key
	})

	s = html.EscapeString(s)

	// Bold.
	boldRe := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")

	// Italic.
	italicRe := regexp.MustCompile(`\*([^*]+)\*`)
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")

	// Links [text](url).
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)

	// Restore code placeholders.
	for k, v := range placeholders {
		s = strings.ReplaceAll(s, html.EscapeString(k), v)
	}

	return s
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
