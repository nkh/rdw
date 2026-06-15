// Package cycle implements focus cycle automation for wall-screen / dashboard
// rotation. A Cycle holds an ordered list of window names and advances through
// them at a configurable interval, notifying callers via a channel.
package cycle

import (
	"context"
	"fmt"
	"time"
)

// Event carries the name of the window that should be focused next.
type Event struct {
	Window string
}

// Cycle rotates focus across a list of windows at a fixed interval.
type Cycle struct {
	windows  []string
	interval time.Duration
	index    int
}

// New returns a Cycle for the given window names and dwell interval.
func New(windows []string, interval time.Duration) (*Cycle, error) {
	if len(windows) == 0 {
		return nil, fmt.Errorf("cycle: window list must not be empty")
	}

	if interval <= 0 {
		return nil, fmt.Errorf("cycle: interval must be positive")
	}

	cp := make([]string, len(windows))
	copy(cp, windows)

	return &Cycle{windows: cp, interval: interval}, nil
}

// Run starts the rotation loop. Events are sent on the returned channel.
// The loop stops when ctx is cancelled; the channel is closed on exit.
func (c *Cycle) Run(ctx context.Context) <-chan Event {
	ch := make(chan Event, 1)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// Send the first window immediately.
		select {
		case ch <- Event{Window: c.windows[c.index]}:
		case <-ctx.Done():
			return
		}

		for {
			select {
			case <-ticker.C:
				c.index = (c.index + 1) % len(c.windows)
				select {
				case ch <- Event{Window: c.windows[c.index]}:
				default: // drop if consumer is slow
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// Windows returns the configured window list.
func (c *Cycle) Windows() []string {
	cp := make([]string, len(c.windows))
	copy(cp, c.windows)

	return cp
}

// Interval returns the configured dwell interval.
func (c *Cycle) Interval() time.Duration { return c.interval }
