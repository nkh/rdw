// Package mirror tees a line stream to a secondary sink: a file/FIFO or a
// shell command's stdin. Used by rdw pipe --forward-to-file and
// --forward-to-cmd.
package mirror

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Sink is a write-closer that receives mirrored lines.
type Sink interface {
	io.WriteCloser
}

// FileSync opens path for appending (creating if absent) and returns a Sink.
func FileSync(path string) (Sink, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("mirror: open %q: %w", path, err)
	}

	return f, nil
}

// CmdSink starts shell (via sh -c) and returns a Sink backed by its stdin.
// The underlying process is started immediately; it runs until the Sink is
// closed or exits on its own.
type CmdSink struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

// CmdSync starts cmdStr as a shell command and returns a Sink whose writes
// are delivered to the command's stdin.
func CmdSync(cmdStr string) (Sink, error) {
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mirror: stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mirror: start %q: %w", cmdStr, err)
	}

	return &CmdSink{cmd: cmd, stdin: stdin}, nil
}

func (s *CmdSink) Write(p []byte) (int, error) { return s.stdin.Write(p) }

func (s *CmdSink) Close() error {
	err := s.stdin.Close()
	_ = s.cmd.Wait()

	return err
}

// Tee wraps r so that every line read is also written to sink with a trailing
// newline. Returns a new io.Reader; the caller reads from it as normal.
func Tee(r io.Reader, sink Sink) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				data := buf[:n]
				_, _ = pw.Write(data)
				_, _ = sink.Write(data)
			}

			if err != nil {
				_ = pw.CloseWithError(err)
				_ = sink.Close()
				return
			}
		}
	}()

	return pr
}
