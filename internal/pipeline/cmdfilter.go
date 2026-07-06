package pipeline

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/nkh/rdw/internal/kvstore"
)

// CmdFilter wraps a shell command as a pipeline Filter. Each line is written
// to the command's stdin; the command's stdout line is the transformed output.
// Lines for which the command produces no output are dropped.
//
// The subprocess is started once and reused for the lifetime of the filter.
// If the subprocess exits, subsequent lines are passed through unchanged.
//
// When a KV store is supplied via WithKV, each invocation re-spawns the
// subprocess with the current KV snapshot injected as environment variables
// using original key names. The subprocess is read-only with respect to KV.
type CmdFilter struct {
	mu      sync.Mutex
	cmdStr  string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	dead    bool
	kv      *kvstore.Store // nil = no KV injection
}

// NewCmdFilter starts cmdStr via sh -c and returns a Filter backed by it.
func NewCmdFilter(cmdStr string) (*CmdFilter, error) {
	f := &CmdFilter{cmdStr: cmdStr}
	if err := f.start(nil) ; err != nil {
		return nil, err
	}
	return f, nil
}

// NewCmdFilterWithKV is like NewCmdFilter but injects KV into the environment.
func NewCmdFilterWithKV(cmdStr string, kv *kvstore.Store) (*CmdFilter, error) {
	f := &CmdFilter{cmdStr: cmdStr, kv: kv}
	if err := f.start(kv) ; err != nil {
		return nil, err
	}
	return f, nil
}

// start spawns the subprocess, optionally injecting the current KV snapshot.
func (f *CmdFilter) start(kv *kvstore.Store) error {
	cmd := exec.Command("sh", "-c", f.cmdStr)

	if kv != nil {
		for k, v := range kv.Snapshot() {
			cmd.Env = append(cmd.Env, k.String()+"="+v)
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("cmdfilter: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("cmdfilter: stdout pipe: %w", err)
	}

	if err := cmd.Start() ; err != nil {
		return fmt.Errorf("cmdfilter: start %q: %w", f.cmdStr, err)
	}

	f.cmd     = cmd
	f.stdin   = stdin
	f.scanner = bufio.NewScanner(stdout)
	f.dead    = false

	return nil
}

// Filter implements the Filter function type. It writes line to the subprocess
// stdin and reads one line from stdout. If the subprocess has died, the
// original line is returned unchanged.
//
// When KV is configured, the subprocess is restarted with a fresh KV snapshot
// on each line so filters always see current values.
func (f *CmdFilter) Filter(line string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Restart dead process if KV is configured (fresh env per line).
	if f.dead && f.kv != nil {
		if err := f.start(f.kv) ; err != nil {
			return line, true
		}
	}

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
		// If KV: will restart on next call.
		if f.kv != nil {
			return line, true
		}
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
