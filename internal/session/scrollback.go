// Package session — scrollback buffer implementation.
package session

import "sync"

const (
	DefaultScrollbackCap = 10_000
	MaxScrollbackCap     = 100_000
)

// ScrollbackBuffer is a bounded circular line buffer. Lines exceeding the cap
// are discarded from the oldest end.
type ScrollbackBuffer struct {
	mu   sync.RWMutex
	buf  []string
	head int // index of the oldest entry
	size int // current number of stored lines
	cap  int // maximum number of lines
}

// NewScrollbackBuffer creates a buffer with the given cap.
// If cap <= 0, DefaultScrollbackCap is used.
// If cap > MaxScrollbackCap, MaxScrollbackCap is used.
func NewScrollbackBuffer(cap int) *ScrollbackBuffer {
	if cap <= 0 {
		cap = DefaultScrollbackCap
	}

	if cap > MaxScrollbackCap {
		cap = MaxScrollbackCap
	}

	return &ScrollbackBuffer{
		buf: make([]string, cap),
		cap: cap,
	}
}

// Append adds a line to the buffer, discarding the oldest line if the
// buffer is at capacity.
func (b *ScrollbackBuffer) Append(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.size < b.cap {
		b.buf[(b.head+b.size)%b.cap] = line
		b.size++

		return
	}

	// Overwrite the oldest entry and advance head.
	b.buf[b.head] = line
	b.head = (b.head + 1) % b.cap
}

// Lines returns a snapshot of all stored lines in insertion order.
func (b *ScrollbackBuffer) Lines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.size == 0 {
		return nil
	}

	out := make([]string, b.size)

	for i := range b.size {
		out[i] = b.buf[(b.head+i)%b.cap]
	}

	return out
}

// Len returns the number of lines currently stored.
func (b *ScrollbackBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.size
}

// Cap returns the maximum number of lines the buffer can hold.
func (b *ScrollbackBuffer) Cap() int {
	return b.cap
}

// Clear discards all stored lines.
func (b *ScrollbackBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.head = 0
	b.size = 0
}
