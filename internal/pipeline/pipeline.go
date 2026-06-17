// Package pipeline implements the per-stream data relay.
//
// A Pipeline reads lines from a reader, applies optional control sequence
// dispatch, and forwards rendered lines to registered sinks. The caller
// provides the KV store and scrollback buffer; Pipeline owns neither.
package pipeline

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/nkh/rdw/internal/control"
	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/session"
)

// Sink is a function that receives a fully processed line for rendering.
type Sink func(targetID session.TargetID, line string)

// ControlHandler is called for non-verbatim control sequences.
type ControlHandler func(targetID session.TargetID, seq control.Sequence)

// Options configures a Pipeline.
type Options struct {
	// TimestampEnabled prepends an RFC3339 timestamp to each output line.
	TimestampEnabled bool

	// MaxFilterStages is the maximum number of filter stages. 0 means unlimited.
	MaxFilterStages int
}

// Pipeline relays lines from a single source to registered sinks.
type Pipeline struct {
	mu             sync.RWMutex
	targetID       session.TargetID
	opts           Options
	sinks          []Sink
	controlHandler ControlHandler
	kv             *kvstore.Store
	scrollback     *session.ScrollbackBuffer
	filters        []Filter
}

// Filter is an external transformation applied to each line before sinks receive it.
// It returns the transformed line, or ("", false) to drop the line entirely.
type Filter func(line string) (string, bool)

// New creates a Pipeline for the given target.
func New(
	targetID session.TargetID,
	opts Options,
	kv *kvstore.Store,
	scrollback *session.ScrollbackBuffer,
) *Pipeline {
	return &Pipeline{
		targetID:   targetID,
		opts:       opts,
		kv:         kv,
		scrollback: scrollback,
	}
}

// AddSink registers a sink. Safe to call concurrently.
func (p *Pipeline) AddSink(s Sink) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.sinks = append(p.sinks, s)
}

// SetControlHandler sets the handler called for control sequences.
func (p *Pipeline) SetControlHandler(h ControlHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.controlHandler = h
}

// AddFilter appends a filter stage. Returns an error if the filter chain is
// already at the configured maximum.
func (p *Pipeline) AddFilter(f Filter) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.opts.MaxFilterStages > 0 && len(p.filters) >= p.opts.MaxFilterStages {
		return fmt.Errorf("filter chain for target %q is at maximum of %d stages",
			p.targetID, p.opts.MaxFilterStages)
	}

	p.filters = append(p.filters, f)

	return nil
}

// SetTimestamp enables or disables timestamp prepending at runtime.
func (p *Pipeline) SetTimestamp(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.opts.TimestampEnabled = enabled
}

// Run reads lines from r until ctx is cancelled or r returns io.EOF.
// Each line is processed and dispatched to sinks or the control handler.
func (p *Pipeline) Run(ctx context.Context, r io.Reader) error {
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := p.processLine(scanner.Text()); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// processLine dispatches a single line through the full pipeline.
func (p *Pipeline) processLine(raw string) error {
	// Control sequence check before any transformation.
	if seq, ok := control.Parse(raw); ok {
		if seq.IsVerbatim() {
			// Strip the "v:" prefix and treat payload as regular content.
			return p.dispatchContent(seq.Payload)
		}

		// Base64 lines are content, not control — decode and dispatch.
		if seq.Kind == control.KindBase64 {
			return p.dispatchContent("b64:" + seq.Payload)
		}

		// Handle KV sequences inline; forward everything else to the handler.
		if seq.Kind == control.KindKV {
			if err := p.applyKVSequence(seq); err != nil {
				// Non-fatal: log at warn level in a production system.
				// Here we return the error to make it observable in tests.
				return err
			}

			return nil
		}

		p.mu.RLock()
		h := p.controlHandler
		p.mu.RUnlock()

		if h != nil {
			h(p.targetID, seq)
		}

		return nil
	}

	return p.dispatchContent(raw)
}

// dispatchContent applies filters, optional timestamp, scrollback, and sinks.
func (p *Pipeline) dispatchContent(line string) error {
	p.mu.RLock()
	filters := p.filters
	tsEnabled := p.opts.TimestampEnabled
	sinks := p.sinks
	p.mu.RUnlock()

	// Apply binary decoding: lines that are valid base64 with a "b64:" prefix
	// are decoded transparently.
	line = maybeDecodeBase64(line)

	// Apply filter chain.
	for _, f := range filters {
		var keep bool
		line, keep = f(line)

		if !keep {
			return nil
		}
	}

	if tsEnabled {
		line = time.Now().UTC().Format(time.RFC3339) + " " + line
	}

	p.scrollback.Append(line)

	for _, s := range sinks {
		s(p.targetID, line)
	}

	return nil
}

// applyKVSequence writes all key=value pairs from a KV control sequence into
// the store.
func (p *Pipeline) applyKVSequence(seq control.Sequence) error {
	pairs, err := seq.KVPairs()
	if err != nil {
		return fmt.Errorf("parsing KV sequence: %w", err)
	}

	for _, pair := range pairs {
		k, err := kvstore.ParseKey(pair[0])
		if err != nil {
			return fmt.Errorf("invalid KV key %q: %w", pair[0], err)
		}

		if err := p.kv.Set(k, pair[1]); err != nil {
			return fmt.Errorf("setting KV key %q: %w", pair[0], err)
		}
	}

	return nil
}

// maybeDecodeBase64 decodes a "b64:<payload>" prefixed line.
// Returns the original line if the prefix is absent or decoding fails.
func maybeDecodeBase64(line string) string {
	const prefix = "b64:"

	if !strings.HasPrefix(line, prefix) {
		return line
	}

	decoded, err := base64.StdEncoding.DecodeString(line[len(prefix):])
	if err != nil {
		return line
	}

	return string(decoded)
}

// Scrollback returns the scrollback buffer associated with this pipeline.
func (p *Pipeline) Scrollback() *session.ScrollbackBuffer {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.scrollback
}
