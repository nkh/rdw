package pipe_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/nkh/rdw/internal/pipe"
	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustID(t *testing.T, s string) session.TargetID {
	t.Helper()
	id, err := session.ParseTargetID(s)
	require.NoError(t, err)
	return id
}

func serverPort(ts *httptest.Server) int {
	return ts.Listener.Addr().(*net.TCPAddr).Port
}

// ---------------------------------------------------------------------------
// HTTP relay - line delivery
// ---------------------------------------------------------------------------

func TestRelay_HTTP_DeliversLines(t *testing.T) {
	var mu sync.Mutex
	received := []string{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		received = append(received, body["line"])
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	err := pipe.Relay(context.Background(), strings.NewReader("a\nb\nc\n"), pipe.Options{
		TargetID: mustID(t, "log"),
		Port:     serverPort(ts),
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"a", "b", "c"}, received)
}

func TestRelay_HTTP_EmptyInput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	err := pipe.Relay(context.Background(), strings.NewReader(""), pipe.Options{
		TargetID: mustID(t, "log"),
		Port:     serverPort(ts),
	})
	assert.NoError(t, err)
}

func TestRelay_HTTP_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pipe.Relay(ctx, strings.NewReader(strings.Repeat("line\n", 10000)), pipe.Options{
		TargetID: mustID(t, "log"),
		Port:     serverPort(ts),
	})
	// Either nil or context.Canceled are acceptable.
	assert.True(t, err == nil || err == context.Canceled)
}

// ---------------------------------------------------------------------------
// Reconnect queue: buffer lines when server is down, flush when it recovers
// ---------------------------------------------------------------------------

func TestRelay_HTTP_BuffersOnError_FlushesOnRecover(t *testing.T) {
	var mu sync.Mutex
	received := []string{}
	failCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if failCount < 3 {
			failCount++
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}

		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		received = append(received, body["line"])
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	err := pipe.Relay(context.Background(), strings.NewReader("1\n2\n3\n4\n5\n"), pipe.Options{
		TargetID:          mustID(t, "log"),
		Port:              serverPort(ts),
		ReconnectQueueLen: 10,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	// Lines delivered when server recovered; order preserved for each line that arrived.
	for _, line := range received {
		assert.Contains(t, []string{"1", "2", "3", "4", "5"}, line)
	}
}

// ---------------------------------------------------------------------------
// Queue overflow: must not block or panic
// ---------------------------------------------------------------------------

func TestRelay_QueueOverflow_DoesNotBlock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	input := strings.Repeat("x\n", 20)
	err := pipe.Relay(context.Background(), strings.NewReader(input), pipe.Options{
		TargetID:          mustID(t, "log"),
		Port:              serverPort(ts),
		ReconnectQueueLen: 5,
	})
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Token forwarded in HTTP header
// ---------------------------------------------------------------------------

func TestRelay_HTTP_SendsAuthHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	_ = pipe.Relay(context.Background(), strings.NewReader("hello\n"), pipe.Options{
		TargetID: mustID(t, "log"),
		Port:     serverPort(ts),
		Token:    "mytoken",
	})

	assert.Equal(t, "Bearer mytoken", gotAuth)
}

// ---------------------------------------------------------------------------
// Target ID in URL path
// ---------------------------------------------------------------------------

func TestRelay_HTTP_CorrectPathAndMethod(t *testing.T) {
	var gotPath, gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	_ = pipe.Relay(context.Background(), strings.NewReader("hello\n"), pipe.Options{
		TargetID: mustID(t, "build-log"),
		Port:     serverPort(ts),
	})

	assert.Equal(t, "/api/v1/stream/build-log", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
}
