package pipeline_test

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nkh/rdw/internal/control"
	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/pipeline"
	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeTarget(t *testing.T) session.TargetID {
	t.Helper()
	id, err := session.ParseTargetID("test-pane")
	require.NoError(t, err)
	return id
}

func makePipeline(t *testing.T, opts pipeline.Options) (*pipeline.Pipeline, *kvstore.Store, *session.ScrollbackBuffer) {
	t.Helper()

	kv := kvstore.New()
	sb := session.NewScrollbackBuffer(1000)
	id := makeTarget(t)
	p := pipeline.New(id, opts, kv, sb)

	return p, kv, sb
}

func collect(p *pipeline.Pipeline) *[]string {
	var mu sync.Mutex
	lines := &[]string{}
	p.AddSink(func(_ session.TargetID, line string) {
		mu.Lock()
		*lines = append(*lines, line)
		mu.Unlock()
	})
	return lines
}

func runLines(t *testing.T, p *pipeline.Pipeline, input string) {
	t.Helper()
	err := p.Run(context.Background(), strings.NewReader(input))
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Basic relay
// ---------------------------------------------------------------------------

func TestPipeline_RelaysPlainLines(t *testing.T) {
	p, _, sb := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	runLines(t, p, "alpha\nbeta\ngamma\n")

	assert.Equal(t, []string{"alpha", "beta", "gamma"}, *lines)
	assert.Equal(t, 3, sb.Len())
}

func TestPipeline_EmptyInput(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	runLines(t, p, "")

	assert.Empty(t, *lines)
}

// ---------------------------------------------------------------------------
// Control sequences
// ---------------------------------------------------------------------------

func TestPipeline_VerbatimPassesThrough(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	runLines(t, p, "v:=:key=value\n")

	assert.Equal(t, []string{"=:key=value"}, *lines)
}

func TestPipeline_ControlSequenceDispatchedToHandler(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})

	var got []control.Sequence
	var mu sync.Mutex
	p.SetControlHandler(func(_ session.TargetID, seq control.Sequence) {
		mu.Lock()
		got = append(got, seq)
		mu.Unlock()
	})

	runLines(t, p, "q:\nc:\nt:\n")

	assert.Len(t, got, 3)
	assert.Equal(t, control.KindQuit, got[0].Kind)
	assert.Equal(t, control.KindClear, got[1].Kind)
	assert.Equal(t, control.KindTimestamp, got[2].Kind)
}

func TestPipeline_KVSequenceWritesToStore(t *testing.T) {
	p, kv, _ := makePipeline(t, pipeline.Options{})

	runLines(t, p, "=:stage=build;status=passing\n")

	stageKey, _ := kvstore.ParseKey("stage")
	statusKey, _ := kvstore.ParseKey("status")

	v, ok := kv.Get(stageKey)
	assert.True(t, ok)
	assert.Equal(t, "build", v)

	v, ok = kv.Get(statusKey)
	assert.True(t, ok)
	assert.Equal(t, "passing", v)
}

func TestPipeline_KVSequenceNotForwardedToSink(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	runLines(t, p, "=:key=val\nnormal line\n")

	assert.Equal(t, []string{"normal line"}, *lines)
}

// ---------------------------------------------------------------------------
// Timestamp
// ---------------------------------------------------------------------------

func TestPipeline_TimestampPrependsRFC3339(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{TimestampEnabled: true})
	lines := collect(p)

	runLines(t, p, "hello\n")

	require.Len(t, *lines, 1)
	// RFC3339 prefix: "2006-01-02T15:04:05Z"
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z hello$`, (*lines)[0])
}

func TestPipeline_SetTimestamp_Toggle(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	runLines(t, p, "no-ts\n")
	p.SetTimestamp(true)
	runLines(t, p, "with-ts\n")

	require.Len(t, *lines, 2)
	assert.Equal(t, "no-ts", (*lines)[0])
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T`, (*lines)[1])
}

// ---------------------------------------------------------------------------
// Base64 decoding
// ---------------------------------------------------------------------------

func TestPipeline_Base64Decode(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	encoded := base64.StdEncoding.EncodeToString([]byte("binary payload"))
	runLines(t, p, "b64:"+encoded+"\n")

	require.Len(t, *lines, 1)
	assert.Equal(t, "binary payload", (*lines)[0])
}

func TestPipeline_Base64_InvalidPayload_PassesThrough(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	runLines(t, p, "b64:!!!notbase64!!!\n")

	require.Len(t, *lines, 1)
	assert.Equal(t, "b64:!!!notbase64!!!", (*lines)[0])
}

// ---------------------------------------------------------------------------
// Filter chain
// ---------------------------------------------------------------------------

func TestPipeline_FilterTransformsLine(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{MaxFilterStages: 8})
	lines := collect(p)

	err := p.AddFilter(func(line string) (string, bool) {
		return strings.ToUpper(line), true
	})
	require.NoError(t, err)

	runLines(t, p, "hello\n")

	assert.Equal(t, []string{"HELLO"}, *lines)
}

func TestPipeline_FilterDropsLine(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{MaxFilterStages: 8})
	lines := collect(p)

	err := p.AddFilter(func(line string) (string, bool) {
		return "", false // drop everything
	})
	require.NoError(t, err)

	runLines(t, p, "hello\nworld\n")

	assert.Empty(t, *lines)
}

func TestPipeline_FilterChainMaxEnforced(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{MaxFilterStages: 2})

	noop := func(line string) (string, bool) { return line, true }

	require.NoError(t, p.AddFilter(noop))
	require.NoError(t, p.AddFilter(noop))
	err := p.AddFilter(noop)
	assert.Error(t, err)
}

func TestPipeline_MultipleFiltersChained(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{MaxFilterStages: 8})
	lines := collect(p)

	_ = p.AddFilter(func(l string) (string, bool) { return strings.TrimSpace(l), true })
	_ = p.AddFilter(func(l string) (string, bool) { return "[" + l + "]", true })

	runLines(t, p, "  hello  \n")

	assert.Equal(t, []string{"[hello]"}, *lines)
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestPipeline_ContextCancellation(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})
	lines := collect(p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// A large input that would hang if cancellation were not respected
	input := strings.Repeat("line\n", 10_000)
	_ = p.Run(ctx, strings.NewReader(input))

	// We can't assert exact count but it must not block indefinitely
	_ = lines
}

// ---------------------------------------------------------------------------
// Multiple sinks
// ---------------------------------------------------------------------------

func TestPipeline_MultipleSinks(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{})

	var s1, s2 []string
	p.AddSink(func(_ session.TargetID, l string) { s1 = append(s1, l) })
	p.AddSink(func(_ session.TargetID, l string) { s2 = append(s2, l) })

	runLines(t, p, "hello\n")

	assert.Equal(t, []string{"hello"}, s1)
	assert.Equal(t, []string{"hello"}, s2)
}

// ---------------------------------------------------------------------------
// Scrollback integration
// ---------------------------------------------------------------------------

func TestPipeline_ScrollbackReceivesLines(t *testing.T) {
	p, _, sb := makePipeline(t, pipeline.Options{})

	runLines(t, p, "a\nb\nc\n")

	assert.Equal(t, 3, sb.Len())
	assert.Equal(t, []string{"a", "b", "c"}, sb.Lines())
}

func TestPipeline_ControlSequencesNotInScrollback(t *testing.T) {
	p, _, sb := makePipeline(t, pipeline.Options{})

	// KV sequence and quit — neither should reach scrollback
	runLines(t, p, "=:k=v\nq:\nnormal\n")

	assert.Equal(t, 1, sb.Len())
	assert.Equal(t, "normal", sb.Lines()[0])
}

// ---------------------------------------------------------------------------
// Sink target ID
// ---------------------------------------------------------------------------

func TestPipeline_SinkReceivesCorrectTargetID(t *testing.T) {
	id, _ := session.ParseTargetID("my-target")
	kv := kvstore.New()
	sb := session.NewScrollbackBuffer(100)
	p := pipeline.New(id, pipeline.Options{}, kv, sb)

	var got session.TargetID
	p.AddSink(func(tid session.TargetID, _ string) { got = tid })

	runLines(t, p, "hello\n")

	assert.Equal(t, id, got)
}

// ---------------------------------------------------------------------------
// Timing safety
// ---------------------------------------------------------------------------

func TestPipeline_TimestampIsUTC(t *testing.T) {
	p, _, _ := makePipeline(t, pipeline.Options{TimestampEnabled: true})

	var got string
	p.AddSink(func(_ session.TargetID, l string) { got = l })

	before := time.Now().UTC()
	runLines(t, p, "msg\n")
	after := time.Now().UTC()

	// Extract timestamp prefix
	parts := strings.SplitN(got, " ", 2)
	require.Len(t, parts, 2)

	ts, err := time.Parse(time.RFC3339, parts[0])
	require.NoError(t, err)

	assert.True(t, !ts.Before(before.Truncate(time.Second)))
	assert.True(t, !ts.After(after.Add(time.Second)))
}
