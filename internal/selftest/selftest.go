// Package selftest implements the rdw selftest command.
//
// It runs an in-process suite covering all core subsystems. No server process
// or browser is required. Exits 0 on success, non-zero on any failure.
package selftest

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/nkh/rdw/internal/auth"
	"github.com/nkh/rdw/internal/config"
	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/layout"
	"github.com/nkh/rdw/internal/pipeline"
	"github.com/nkh/rdw/internal/router"
	"github.com/nkh/rdw/internal/server"
	"github.com/nkh/rdw/internal/session"
)

// Result holds the outcome of a selftest run.
type Result struct {
	Passed bool
	Checks []Check
}

// Check is the result of a single assertion.
type Check struct {
	Name   string
	Passed bool
	Detail string
}

// Run executes all built-in self-tests and returns the aggregate result.
func Run(ctx context.Context, w io.Writer) Result {
	checks := []Check{
		checkTargetID(),
		checkKVStore(),
		checkControlSequence(),
		checkPipelineRelay(),
		checkBase64Decode(),
		checkScrollbackOverflow(),
		checkLayoutParse(),
		checkRouterBasic(),
		checkSessionManager(),
		checkAuthToken(),
		checkServerPing(),
	}

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
// Checks
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
	store.Delete(k)
	_, ok = store.Get(k)
	if ok {
		return Check{"KV store", false, "delete did not remove key"}
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
		return Check{"Control sequence dispatch", false,
			fmt.Sprintf("KV selftest=%q ok=%v", v, ok)}
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

	if err := p.Run(ctx, strings.NewReader("line1\nline2\nline3\n")); err != nil {
		return Check{"Pipeline relay", false, err.Error()}
	}
	if len(received) != 3 || strings.Join(received, ",") != "line1,line2,line3" {
		return Check{"Pipeline relay", false, fmt.Sprintf("got %v", received)}
	}
	return Check{Name: "Pipeline relay", Passed: true}
}

func checkBase64Decode() Check {
	encoded := base64.StdEncoding.EncodeToString([]byte("hello"))
	store := kvstore.New()
	sb := session.NewScrollbackBuffer(100)
	id, _ := session.ParseTargetID("selftest-b64")
	p := pipeline.New(id, pipeline.Options{}, store, sb)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := p.Run(ctx, strings.NewReader("b64:"+encoded+"\n")); err != nil {
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
	if len(lines) != 3 || lines[0] != "line2" || lines[2] != "line4" {
		return Check{"Scrollback overflow", false, fmt.Sprintf("got %v", lines)}
	}
	return Check{Name: "Scrollback overflow", Passed: true}
}

func checkLayoutParse() Check {
	yaml := `
schema_version: 1
name: selftest
windows:
  - name: build
    panes:
      - target_id: stdout
      - target_id: stderr
        split: h
        size: 30%
`
	l, err := layout.Parse([]byte(yaml))
	if err != nil {
		return Check{"Layout parse", false, err.Error()}
	}
	if len(l.Windows) != 1 || l.Windows[0].Name != "build" {
		return Check{"Layout parse", false, "unexpected window structure"}
	}
	if len(l.Windows[0].Panes) != 2 {
		return Check{"Layout parse", false, "expected 2 panes"}
	}
	return Check{Name: "Layout parse", Passed: true}
}

func checkRouterBasic() Check {
	kv := kvstore.New()
	var got []string
	r := router.New(kv, func(id session.TargetID, line string) {
		got = append(got, id.String()+":"+line)
	}, router.Options{AllowUnassigned: false})

	id, _ := session.ParseTargetID("st-pane")
	if _, err := r.Register(id, 100); err != nil {
		return Check{"Router basic", false, err.Error()}
	}
	if err := r.Route(id, "hello"); err != nil {
		return Check{"Router basic", false, err.Error()}
	}
	if len(got) != 1 || got[0] != "st-pane:hello" {
		return Check{"Router basic", false, fmt.Sprintf("got %v", got)}
	}

	// Unassigned with no AllowUnassigned should error.
	id2, _ := session.ParseTargetID("missing")
	if err := r.Route(id2, "x"); err == nil {
		return Check{"Router basic", false, "expected error for unregistered target"}
	}
	return Check{Name: "Router basic", Passed: true}
}

func checkSessionManager() Check {
	m := session.NewManager(kvstore.New())
	if err := m.CreateWindow("w1"); err != nil {
		return Check{"Session manager", false, err.Error()}
	}
	if err := m.CreateWindow("w2"); err != nil {
		return Check{"Session manager", false, err.Error()}
	}
	if err := m.FocusWindow("w2"); err != nil {
		return Check{"Session manager", false, err.Error()}
	}
	if m.ActiveWindow().Name != "w2" {
		return Check{"Session manager", false, "wrong active window"}
	}
	if err := m.CloseWindow("w1"); err != nil {
		return Check{"Session manager", false, err.Error()}
	}
	if len(m.Windows()) != 1 {
		return Check{"Session manager", false, "expected 1 window after close"}
	}
	return Check{Name: "Session manager", Passed: true}
}

func checkAuthToken() Check {
	store := auth.NewStore()
	plain, tok, err := store.Create(auth.CreateOptions{Expiry: time.Hour})
	if err != nil {
		return Check{"Auth token", false, err.Error()}
	}
	if plain == tok.Hash {
		return Check{"Auth token", false, "plaintext equals hash"}
	}
	got, ok := store.Verify(plain)
	if !ok || got.ID != tok.ID {
		return Check{"Auth token", false, "verify failed for valid token"}
	}
	if _, ok := store.Verify("wrongtoken"); ok {
		return Check{"Auth token", false, "verify succeeded for wrong token"}
	}
	store.Revoke(tok.ID)
	if _, ok := store.Verify(plain); ok {
		return Check{"Auth token", false, "verify succeeded after revoke"}
	}
	return Check{Name: "Auth token", Passed: true}
}

func checkServerPing() Check {
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	s := server.New(cfg, server.Options{SessionID: "selftest-ping"})
	ts := httptest.NewServer(s.HTTPHandler())
	defer ts.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(ts.URL + "/api/v1/ping")
	if err != nil {
		return Check{"Server ping", false, err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Check{"Server ping", false, fmt.Sprintf("status %d", resp.StatusCode)}
	}
	return Check{Name: "Server ping", Passed: true}
}
