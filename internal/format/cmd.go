package format

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// KVSnapshot is a read-only view of the session KV store passed to CmdFormatter.
// Using a plain map avoids importing kvstore from the format package.
type KVSnapshot map[string]string

// CmdFormatter invokes an external shell command as a formatter.
// The scrollback lines are written to the command's stdin (one per line).
// The command's stdout is treated as HTML and wrapped in a sandboxed iframe
// so injected scripts cannot reach the parent page.
//
// The current KV snapshot is injected into the subprocess environment with
// original key names and values. The subprocess is read-only with respect to KV.
//
// CmdFormatter is registered per-session or per-pane, not compiled in.
type CmdFormatter struct {
	name   string
	cmdStr string
	kv     KVSnapshot // may be nil
}

// NewCmdFormatter returns a CmdFormatter. kv may be nil.
func NewCmdFormatter(name, cmdStr string, kv KVSnapshot) *CmdFormatter {
	return &CmdFormatter{name: name, cmdStr: cmdStr, kv: kv}
}

func (f *CmdFormatter) Name() string { return f.name }

// Format runs the external command with lines on stdin and returns sandboxed HTML.
func (f *CmdFormatter) Format(lines []string) (string, error) {
	cmd := exec.Command("sh", "-c", f.cmdStr)

	for k, v := range f.kv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run() ; err != nil {
		return "", fmt.Errorf(
			"cmdformatter %q: %w\nstderr: %s",
			f.name, err, stderr.String(),
		)
	}

	return sandboxHTML(stdout.String()), nil
}

// sandboxHTML wraps arbitrary HTML in a sandboxed iframe so inline scripts
// in formatter output cannot access the parent rdw SPA.
func sandboxHTML(html string) string {
	escaped := escapeHTML(html)

	// Use srcdoc for inline content; sandbox blocks scripts by default.
	// allow-same-origin is intentionally omitted.
	return `<iframe class="rdw-fmt-sandbox"` +
		` sandbox="allow-popups"` +
		` srcdoc="` + strings.ReplaceAll(escaped, `"`, `&quot;`) + `"` +
		` style="width:100%;height:100%;border:none;background:#fff">` +
		`</iframe>`
}
