// Package router maps Target IDs to active pipelines and manages their lifecycle.
package router

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/nkh/rdw/internal/control"
	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/pipeline"
	"github.com/nkh/rdw/internal/session"
)

// PipelineSink is called for each rendered line with the target ID and line text.
type PipelineSink = pipeline.Sink

// Router manages the set of active pipelines for a session.
type Router struct {
	mu       sync.RWMutex
	pipes    map[session.TargetID]*pipeline.Pipeline
	kv       *kvstore.Store
	sinkFn   PipelineSink
	opts     Options
}

// Options configures the router.
type Options struct {
	// AllowUnassigned creates a new pipeline when data arrives for an unknown Target ID.
	AllowUnassigned bool

	// FilterChainMax is passed through to each pipeline.
	FilterChainMax int

	// DefaultScrollbackCap is used when creating pipelines implicitly.
	DefaultScrollbackCap int
}

// New creates a Router backed by the given KV store.
// sinkFn is called for every rendered output line; typically this broadcasts
// to the WebSocket hub.
func New(kv *kvstore.Store, sinkFn PipelineSink, opts Options) *Router {
	if opts.FilterChainMax == 0 {
		opts.FilterChainMax = 8
	}

	if opts.DefaultScrollbackCap == 0 {
		opts.DefaultScrollbackCap = 10_000
	}

	return &Router{
		pipes:  make(map[session.TargetID]*pipeline.Pipeline),
		kv:     kv,
		sinkFn: sinkFn,
		opts:   opts,
	}
}

// Register creates and registers a pipeline for the given Target ID.
// Returns an error if the ID is already registered.
func (r *Router) Register(id session.TargetID, scrollbackCap int) (*pipeline.Pipeline, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.pipes[id]; exists {
		return nil, fmt.Errorf("target %q already registered", id)
	}

	p := r.newPipeline(id, scrollbackCap)
	r.pipes[id] = p

	return p, nil
}

// Get returns the pipeline for the given Target ID, or (nil, false).
func (r *Router) Get(id session.TargetID) (*pipeline.Pipeline, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.pipes[id]
	return p, ok
}

// Deregister removes and stops the pipeline for the given Target ID.
// No-op if the ID is not registered.
func (r *Router) Deregister(id session.TargetID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pipes, id)
}

// Route sends a single line to the pipeline for id.
// If the ID is unregistered and AllowUnassigned is true, a pipeline is created.
// Otherwise an error is returned.
func (r *Router) Route(id session.TargetID, line string) error {
	p, ok := r.Get(id)

	if !ok {
		if !r.opts.AllowUnassigned {
			return fmt.Errorf("no registered pane for target %q; use --allow-unassigned to create one", id)
		}

		var err error
		p, err = r.Register(id, r.opts.DefaultScrollbackCap)
		if err != nil {
			// Race: another goroutine registered it first.
			p, ok = r.Get(id)
			if !ok {
				return err
			}
		}
	}

	ctx := context.Background()
	return p.Run(ctx, strings.NewReader(line+"\n"))
}

// Stream reads lines from r and routes each to the pipeline for id.
// Blocks until the reader returns io.EOF or an error.
func (r *Router) Stream(ctx context.Context, id session.TargetID, rd io.Reader) error {
	p, ok := r.Get(id)

	if !ok {
		if !r.opts.AllowUnassigned {
			return fmt.Errorf("no registered pane for target %q", id)
		}

		var err error
		p, err = r.Register(id, r.opts.DefaultScrollbackCap)
		if err != nil {
			p, ok = r.Get(id)
			if !ok {
				return err
			}
		}
	}

	return p.Run(ctx, rd)
}

// SetControlHandler sets the control sequence handler on every registered
// pipeline. Must be called before any data arrives.
func (r *Router) SetControlHandler(h func(session.TargetID, control.Sequence)) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.pipes {
		p.SetControlHandler(h)
	}
}

// Targets returns all currently registered Target IDs.
func (r *Router) Targets() []session.TargetID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]session.TargetID, 0, len(r.pipes))
	for id := range r.pipes {
		ids = append(ids, id)
	}

	return ids
}

// Len returns the number of registered pipelines.
func (r *Router) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.pipes)
}

// newPipeline creates a pipeline for the given Target ID.
func (r *Router) newPipeline(id session.TargetID, scrollbackCap int) *pipeline.Pipeline {
	if scrollbackCap <= 0 {
		scrollbackCap = r.opts.DefaultScrollbackCap
	}

	sb := session.NewScrollbackBuffer(scrollbackCap)
	p := pipeline.New(id, pipeline.Options{
		MaxFilterStages: r.opts.FilterChainMax,
	}, r.kv, sb)

	if r.sinkFn != nil {
		p.AddSink(r.sinkFn)
	}

	return p
}
