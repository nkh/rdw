package pipe

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"strings"

	"github.com/nkh/rdw/internal/control"
)

// hybridReader wraps an io.Reader and presents a line-oriented interface.
// In normal mode it behaves like bufio.Scanner, yielding one text line per
// call to Next(). When it encounters an "image:" or "svg:" prefix line it
// switches to binary accumulation mode, reads raw bytes until the matching
// sentinel ("image:end" or "svg:end"), then yields a single synthetic line:
//
//   - image: → "b64:<base64-of-accumulated-bytes>"
//   - svg:   → "svg-data:<raw-utf8-payload>"
//
// The caller sends that synthetic line to the server as a normal text line.
type hybridReader struct {
	scanner *bufio.Scanner
	pending string
	err     error
}

func newHybridReader(r io.Reader) *hybridReader {
	return &hybridReader{scanner: bufio.NewScanner(r)}
}

// NewHybridReaderExported exposes newHybridReader for package tests.
func NewHybridReaderExported(r io.Reader) *hybridReader {
	return newHybridReader(r)
}

// Next advances to the next logical line. Returns false when the input is
// exhausted or an error occurred.
func (h *hybridReader) Next() bool {
	if !h.scanner.Scan() {
		h.err = h.scanner.Err()
		return false
	}

	line := h.scanner.Text()

	switch {
	case strings.HasPrefix(line, "image:") && line != control.SentinelImageEnd:
		h.pending = h.accumBinary(control.SentinelImageEnd)
		return true

	case strings.HasPrefix(line, "svg:") && line != control.SentinelSVGEnd:
		h.pending = h.accumSVG(control.SentinelSVGEnd)
		return true

	default:
		h.pending = line
		return true
	}
}

// Text returns the current logical line.
func (h *hybridReader) Text() string { return h.pending }

// Err returns the first non-EOF error.
func (h *hybridReader) Err() error { return h.err }

// accumBinary reads lines from the shared scanner until sentinel, returns b64 line.
func (h *hybridReader) accumBinary(sentinel string) string {
	var buf bytes.Buffer

	for h.scanner.Scan() {
		line := h.scanner.Text()
		if line == sentinel {
			break
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	return "b64:" + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// accumSVG reads lines from the shared scanner until sentinel, returns svg-data line.
func (h *hybridReader) accumSVG(sentinel string) string {
	var buf strings.Builder

	for h.scanner.Scan() {
		line := h.scanner.Text()
		if line == sentinel {
			break
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	return "svg-data:" + base64.StdEncoding.EncodeToString([]byte(buf.String()))
}
