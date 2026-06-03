package router_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/nkh/rdw/internal/control"
	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/router"
	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeID(t *testing.T, s string) session.TargetID {
	t.Helper()
	id, err := session.ParseTargetID(s)
	require.NoError(t, err)
	return id
}

func newRouter(t *testing.T, allowUnassigned bool) (*router.Router, *[]string) {
	t.Helper()
	kv := kvstore.New()
	received := &[]string{}
	var mu sync.Mutex
	sink := func(id session.TargetID, line string) {
		mu.Lock()
		*received = append(*received, string(id)+":"+line)
		mu.Unlock()
	}
	r := router.New(kv, sink, router.Options{
		AllowUnassigned:      allowUnassigned,
		FilterChainMax:       8,
		DefaultScrollbackCap: 100,
	})
	return r, received
}

// ---------------------------------------------------------------------------
// Register / Get / Deregister
// ---------------------------------------------------------------------------

func TestRouter_Register_Success(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "build-log")

	p, err := r.Register(id, 0)
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, 1, r.Len())
}

func TestRouter_Register_Duplicate(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "build-log")

	_, err := r.Register(id, 0)
	require.NoError(t, err)

	_, err = r.Register(id, 0)
	assert.Error(t, err)
}

func TestRouter_Get_Found(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "pane1")

	_, _ = r.Register(id, 0)
	p, ok := r.Get(id)
	assert.True(t, ok)
	assert.NotNil(t, p)
}

func TestRouter_Get_Missing(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "ghost")

	_, ok := r.Get(id)
	assert.False(t, ok)
}

func TestRouter_Deregister_Removes(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "temp")

	_, _ = r.Register(id, 0)
	assert.Equal(t, 1, r.Len())

	r.Deregister(id)
	assert.Equal(t, 0, r.Len())

	_, ok := r.Get(id)
	assert.False(t, ok)
}

func TestRouter_Deregister_MissingIsNoOp(t *testing.T) {
	r, _ := newRouter(t, false)
	r.Deregister(makeID(t, "nope")) // must not panic
}

func TestRouter_Targets(t *testing.T) {
	r, _ := newRouter(t, false)

	for _, name := range []string{"a", "b", "c"} {
		_, _ = r.Register(makeID(t, name), 0)
	}

	targets := r.Targets()
	assert.Len(t, targets, 3)
}

// ---------------------------------------------------------------------------
// Route
// ---------------------------------------------------------------------------

func TestRouter_Route_Registered(t *testing.T) {
	r, received := newRouter(t, false)
	id := makeID(t, "log")

	_, _ = r.Register(id, 0)
	err := r.Route(id, "hello world")
	require.NoError(t, err)

	assert.Contains(t, *received, "log:hello world")
}

func TestRouter_Route_Unregistered_NoAllow(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "missing")

	err := r.Route(id, "data")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestRouter_Route_Unregistered_WithAllow(t *testing.T) {
	r, received := newRouter(t, true)
	id := makeID(t, "auto")

	err := r.Route(id, "auto-created line")
	require.NoError(t, err)
	assert.Equal(t, 1, r.Len())
	assert.Contains(t, *received, "auto:auto-created line")
}

func TestRouter_Route_KVSequenceWritesToStore(t *testing.T) {
	kv := kvstore.New()
	r := router.New(kv, nil, router.Options{AllowUnassigned: true})
	id := makeID(t, "stream")

	_, _ = r.Register(id, 0)
	err := r.Route(id, "=:build=ok")
	require.NoError(t, err)

	k, _ := kvstore.ParseKey("build")
	v, ok := kv.Get(k)
	assert.True(t, ok)
	assert.Equal(t, "ok", v)
}

// ---------------------------------------------------------------------------
// Stream
// ---------------------------------------------------------------------------

func TestRouter_Stream_DeliversLines(t *testing.T) {
	r, received := newRouter(t, false)
	id := makeID(t, "stdin")

	_, _ = r.Register(id, 0)
	input := "line1\nline2\nline3\n"
	err := r.Stream(context.Background(), id, strings.NewReader(input))
	require.NoError(t, err)

	assert.Contains(t, *received, "stdin:line1")
	assert.Contains(t, *received, "stdin:line2")
	assert.Contains(t, *received, "stdin:line3")
}

func TestRouter_Stream_UnregisteredNoAllow(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "nope")

	err := r.Stream(context.Background(), id, strings.NewReader("data\n"))
	assert.Error(t, err)
}

func TestRouter_Stream_ContextCancellation(t *testing.T) {
	r, _ := newRouter(t, false)
	id := makeID(t, "cancel")
	_, _ = r.Register(id, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return promptly; not block on a large reader.
	big := strings.Repeat("line\n", 100_000)
	_ = r.Stream(ctx, id, strings.NewReader(big))
}

// ---------------------------------------------------------------------------
// Control handler
// ---------------------------------------------------------------------------

func TestRouter_SetControlHandler_AllPipelines(t *testing.T) {
	r, _ := newRouter(t, false)

	ids := []string{"p1", "p2", "p3"}
	for _, name := range ids {
		_, _ = r.Register(makeID(t, name), 0)
	}

	var mu sync.Mutex
	var handled []control.Kind
	r.SetControlHandler(func(_ session.TargetID, seq control.Sequence) {
		mu.Lock()
		handled = append(handled, seq.Kind)
		mu.Unlock()
	})

	for _, name := range ids {
		_ = r.Route(makeID(t, name), "c:")
	}

	assert.Len(t, handled, 3)
}

// ---------------------------------------------------------------------------
// Concurrent safety
// ---------------------------------------------------------------------------

func TestRouter_Concurrent(t *testing.T) {
	r, _ := newRouter(t, true)
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := makeID(t, "pane")
			_, _ = r.Register(id, 0) // first wins; rest get duplicate error
			_ = r.Route(id, "line")
		}(i)
	}

	wg.Wait()
}
