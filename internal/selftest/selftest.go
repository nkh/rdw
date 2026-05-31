// Package selftest implements the rdw selftest command.
//
// It starts a minimal in-process server, sends a known payload through
// the pipeline, and verifies the scrollback buffer received the expected
// output. Exits 0 on success, non-zero on failure.
package selftest

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/pipeline"
	"github.com/nkh/rdw/internal/session"
)

// Result holds the outcome of a selftest run.
type Result struct {
	Passed  bool
	Checks  []Check
}

// Check is the result of a single assertion within the selftest.
type Check struct {
	Name   string
	Passed bool
	Detail string
}

// Run executes all built-in self-tests and returns the aggregate result.
func Run(ctx context.Context, w io.Writer) Result {
	var checks []Check

	checks = append(checks, checkTargetID())
	checks = append(checks, checkKVStore())
	checks = append(checks, checkControlSequence())
	checks = append(checks, checkPipelineRelay())
	checks = append(checks, checkBase64Decode())
	checks = append(checks, checkScrollbackOverflow())

	passed := true

	for _, c := range checks {
		status := "PASS"

		if !c.Passed {
			status = "FAIL"
			passed = false
		}

		line := fmt.Sprintf("[%s] %s", status, c.Name)

		if !c.Passed && c.Detail != "" {
			line += ": " + c.Detail
		}

		fmt.Fprintln(w, line)
	}

	return Result{Passed: passed, Checks: checks}
}

// ---------------------------------------------------------------------------
// Individual checks
// ---------------------------------------------------------------------------

func checkTargetID() Check {
	_, err := session.ParseTargetID("selftest-pane")
	if err != nil {
		return Check{"TargetID validation", false, err.Error()}
	}

	_, err = session.ParseTargetID("-invalid")
	if err == nil {
		return Check{"TargetID validation", false, "expected error for invalid ID"}
	}

	return Check{Name: "TargetID validation", Passed: true}
}

func checkKVStore() Check {
	store := kvstore.New()
	k, err := kvstore.ParseKey("selftest-key")

	if err != nil {
		return Check{"KV store", false, err.Error()}
	}

	if err = store.Set(k, "hello"); err != nil {
		return Check{"KV store", false, err.Error()}
	}

	v, ok := store.Get(k)

	if !ok || v != "hello" {
		return Check{"KV store", false, fmt.Sprintf("got %q want %q", v, "hello")}
	}

	return Check{Name: "KV store", Passed: true}
}

func checkControlSequence() Check {
	store := kvstore.New()
	sb := session.NewScrollbackBuffer(100)
	id, _ := session.ParseTargetID("selftest-ctrl")
	p := pipeline.New(id, pipeline.Options{MaxFilterStages: 8}, store, sb)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := "=:selftest=ok\nnormal line\n"

	if err := p.Run(ctx, strings.NewReader(input)); err != nil {
		return Check{"Control sequence dispatch", false, err.Error()}
	}

	k, _ := kvstore.ParseKey("selftest")
	v, ok := store.Get(k)

	if !ok || v != "ok" {
		return Check{"Control sequence dispatch", false, fmt.Sprintf("KV selftest=%q (ok=%v)", v, ok)}
	}

	if sb.Len() != 1 || sb.Lines()[0] != "normal line" {
		return Check{"Control sequence dispatch", false,
			fmt.Sprintf("scrollback: %v", sb.Lines())}
	}

	return Check{Name: "Control sequence dispatch", Passed: true}
}

func checkPipelineRelay() Check {
	store := kvstore.New()
	sb := session.NewScrollbackBuffer(100)
	id, _ := session.ParseTargetID("selftest-relay")
	p := pipeline.New(id, pipeline.Options{}, store, sb)

	var received []string
	p.AddSink(func(_ session.TargetID, line string) {
		received = append(received, line)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := "line1\nline2\nline3\n"

	if err := p.Run(ctx, strings.NewReader(input)); err != nil {
		return Check{"Pipeline relay", false, err.Error()}
	}

	if len(received) != 3 {
		return Check{"Pipeline relay", false,
			fmt.Sprintf("got %d lines, want 3", len(received))}
	}

	if strings.Join(received, ",") != "line1,line2,line3" {
		return Check{"Pipeline relay", false,
			fmt.Sprintf("got %v", received)}
	}

	return Check{Name: "Pipeline relay", Passed: true}
}

func checkBase64Decode() Check {
	import64 := "b64:aGVsbG8=" // base64("hello")

	store := kvstore.New()
	sb := session.NewScrollbackBuffer(100)
	id, _ := session.ParseTargetID("selftest-b64")
	p := pipeline.New(id, pipeline.Options{}, store, sb)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := p.Run(ctx, strings.NewReader(import64+"\n")); err != nil {
		return Check{"Base64 decode", false, err.Error()}
	}

	lines := sb.Lines()

	if len(lines) != 1 || lines[0] != "hello" {
		return Check{"Base64 decode", false, fmt.Sprintf("got %v", lines)}
	}

	return Check{Name: "Base64 decode", Passed: true}
}

func checkScrollbackOverflow() Check {
	sb := session.NewScrollbackBuffer(3)

	for i := range 5 {
		sb.Append(fmt.Sprintf("line%d", i))
	}

	lines := sb.Lines()

	if len(lines) != 3 {
		return Check{"Scrollback overflow", false,
			fmt.Sprintf("got %d lines, want 3", len(lines))}
	}

	if lines[0] != "line2" || lines[2] != "line4" {
		return Check{"Scrollback overflow", false,
			fmt.Sprintf("got %v", lines)}
	}

	return Check{Name: "Scrollback overflow", Passed: true}
}
