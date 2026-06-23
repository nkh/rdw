package pipeline

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// CmdFilter wraps a shell command as a pipeline Filter. Each line is written
// to the command's stdin; the command's stdout line is the transformed output.
// Lines for which the command produces no output are dropped.
//
// The subprocess is started once and reused for the lifetime of the filter.
// If the subprocess exits, subsequent lines are passed through unchanged.
type CmdFilter struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	dead    bool
}

// NewCmdFilter starts cmdStr via sh -c and returns a Filter backed by it.
func NewCmdFilter(cmdStr string) (*CmdFilter, error) {
	cmd := exec.Command("sh", "-c", cmdStr)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("cmdfilter: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("cmdfilter: stdout pipe: %w", err)
	}

	if err := cmd.Start() ; err != nil {
		return nil, fmt.Errorf("cmdfilter: start %q: %w", cmdStr, err)
	}

	f := &CmdFilter{
		cmd:     cmd,
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
	}

	return f, nil
}

// Filter implements the Filter function type. It writes line to the subprocess
// stdin and reads one line from stdout. If the subprocess has died, the
// original line is returned unchanged.
func (f *CmdFilter) Filter(line string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.dead {
		return line, true
	}

	if _, err := fmt.Fprintln(f.stdin, line) ; err != nil {
		f.dead = true
		return line, true
	}

	if !f.scanner.Scan() {
		f.dead = true
		_ = f.cmd.Wait()
		return line, true
	}

	out := f.scanner.Text()
	if out == "" {
		return "", false
	}

	return out, true
}

// Close shuts down the subprocess.
func (f *CmdFilter) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.dead {
		return nil
	}

	f.dead = true
	_ = f.stdin.Close()
	_ = f.cmd.Wait()

	return nil
}
