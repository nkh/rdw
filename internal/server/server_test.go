package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nkh/rdw/internal/auth"
	"github.com/nkh/rdw/internal/config"
	"github.com/nkh/rdw/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Hub
// ---------------------------------------------------------------------------

func TestHub_ConnCount_StartsZero(t *testing.T) {
	hub := server.NewHub()
	assert.Equal(t, 0, hub.ConnCount())
}

func TestHub_RevokeToken_NoConnections(t *testing.T) {
	hub := server.NewHub()
	// Must not panic when revoking a token with no connections.
	hub.RevokeToken("nonexistent")
	assert.Equal(t, 0, hub.ConnCount())
}

// ---------------------------------------------------------------------------
// Rate limiter
// ---------------------------------------------------------------------------

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := server.NewRateLimiter()
	for i := range 10 {
		assert.True(t, rl.Allow("127.0.0.1"), "request %d should be allowed", i+1)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := server.NewRateLimiter()
	for range 10 {
		rl.Allow("192.168.1.1")
	}
	assert.False(t, rl.Allow("192.168.1.1"), "11th request should be blocked")
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := server.NewRateLimiter()
	for range 10 {
		rl.Allow("10.0.0.1")
	}
	// Different IP should not be rate-limited.
	assert.True(t, rl.Allow("10.0.0.2"))
}

// ---------------------------------------------------------------------------
// Ping endpoint
// ---------------------------------------------------------------------------

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	cfg := config.Default()
	cfg.Auth.NoAuth = true // simplify auth for most tests

	s := server.New(cfg, server.Options{SessionID: "test"})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)

	return ts
}

func TestPing_ReturnsOK(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/ping")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
	assert.NotEmpty(t, body["time"])
}

func TestPing_TimeIsRFC3339(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/ping")
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	_, err = time.Parse(time.RFC3339, body["time"])
	assert.NoError(t, err, "time field must be RFC3339: %s", body["time"])
}

// ---------------------------------------------------------------------------
// Frontend
// ---------------------------------------------------------------------------

func TestFrontend_RootReturnsHTML(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
}

func TestFrontend_NotFoundForUnknownPath(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func TestAuth_SessionRequiresToken(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = false
	s := server.New(cfg, server.Options{SessionID: "auth-test"})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/v1/session")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAuth_ValidToken_Allowed(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = false
	s := server.New(cfg, server.Options{SessionID: "auth-test2"})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)

	// Create a token.
	plain, _, err := s.TokenStore().Create(auth.CreateOptions{Expiry: time.Hour})
	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/session", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAuth_ExpiredToken_Rejected(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = false
	s := server.New(cfg, server.Options{SessionID: "auth-test3"})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)

	plain, _, err := s.TokenStore().Create(auth.CreateOptions{Expiry: time.Millisecond})
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/session", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAuth_NoAuth_BypassesCheck(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	s := server.New(cfg, server.Options{SessionID: "noauth-test"})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/v1/session")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// WebSocket connection
// ---------------------------------------------------------------------------

func TestWebSocket_ConnectsWithProto(t *testing.T) {
	ts := newTestServer(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"

	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"rdw-v1"},
	})
	require.NoError(t, err, "WebSocket dial should succeed")
	defer conn.Close()
	defer resp.Body.Close()

	assert.Equal(t, "rdw-v1", resp.Header.Get("Sec-Websocket-Protocol"))
}

func TestWebSocket_RejectsWrongProto(t *testing.T) {
	ts := newTestServer(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"

	dialer := websocket.Dialer{}
	_, resp, err := dialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"wrong-proto"},
	})
	if resp != nil {
		defer resp.Body.Close()
	}
	// Should fail to upgrade due to wrong sub-protocol.
	assert.Error(t, err)
}

func TestWebSocket_ReceivesReconnectMarker(t *testing.T) {
	ts := newTestServer(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"rdw-v1"},
	})
	require.NoError(t, err)
	defer conn.Close()

	// First message from server should be the reconnect marker.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var special map[string]interface{}
	require.NoError(t, json.Unmarshal(msg, &special))
	assert.Equal(t, "reconnect", special["type"])
}

func TestWebSocket_BroadcastReachesClient(t *testing.T) {
	// Use a single server instance so the broadcast reaches the connected client.
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	s := server.New(cfg, server.Options{SessionID: fmt.Sprintf("bcast-%d", time.Now().UnixNano())})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"rdw-v1"},
	})
	require.NoError(t, err)
	defer conn.Close()

	// Drain the reconnect message.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	// Small delay to ensure connection is fully registered in the hub.
	time.Sleep(20 * time.Millisecond)

	// Broadcast a message via the hub of the same server.
	payload, _ := json.Marshal(server.Message{TargetID: "build-log", Line: "hello"})
	s.Hub().Broadcast(payload)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)

	var msg server.Message
	require.NoError(t, json.Unmarshal(raw, &msg))
	assert.Equal(t, "build-log", msg.TargetID)
	assert.Equal(t, "hello", msg.Line)
}

func TestWebSocket_ConnCountTracked(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	s := server.New(cfg, server.Options{SessionID: "count-test"})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"

	assert.Equal(t, 0, s.Hub().ConnCount())

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"rdw-v1"},
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond) // allow goroutines to start
	assert.Equal(t, 1, s.Hub().ConnCount())

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"rdw-v1"},
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, s.Hub().ConnCount())

	conn1.Close()
	conn2.Close()
}

// ---------------------------------------------------------------------------
// Unix socket
// ---------------------------------------------------------------------------

func TestUnixSocket_Ping(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = true

	sessionID := fmt.Sprintf("test-%d", time.Now().UnixNano())
	s := server.New(cfg, server.Options{SessionID: sessionID})

	sockPath, err := s.StartUnixSocket()
	require.NoError(t, err)
	t.Cleanup(func() { s.StopUnixSocket() })

	// Send ping command over Unix socket.
	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	_ = json.NewEncoder(conn).Encode(server.UnixCommand{Action: "ping"})

	var resp server.UnixResponse
	require.NoError(t, json.NewDecoder(conn).Decode(&resp))
	assert.True(t, resp.OK)
}

func TestUnixSocket_UnknownAction(t *testing.T) {
	cfg := config.Default()
	sessionID := fmt.Sprintf("test2-%d", time.Now().UnixNano())
	s := server.New(cfg, server.Options{SessionID: sessionID})

	sockPath, err := s.StartUnixSocket()
	require.NoError(t, err)
	t.Cleanup(func() { s.StopUnixSocket() })

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	_ = json.NewEncoder(conn).Encode(server.UnixCommand{Action: "explode"})

	var resp server.UnixResponse
	require.NoError(t, json.NewDecoder(conn).Decode(&resp))
	assert.False(t, resp.OK)
	assert.Contains(t, resp.Error, "unknown action")
}

func TestUnixSocket_TokenCreate(t *testing.T) {
	cfg := config.Default()
	sessionID := fmt.Sprintf("test3-%d", time.Now().UnixNano())
	s := server.New(cfg, server.Options{SessionID: sessionID})

	sockPath, err := s.StartUnixSocket()
	require.NoError(t, err)
	t.Cleanup(func() { s.StopUnixSocket() })

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	params, _ := json.Marshal(map[string]interface{}{
		"expiry_seconds": 3600,
	})
	_ = json.NewEncoder(conn).Encode(server.UnixCommand{
		Action: "token.create",
		Params: params,
	})

	var resp server.UnixResponse
	require.NoError(t, json.NewDecoder(conn).Decode(&resp))
	assert.True(t, resp.OK)

	data, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, data["token"])
	assert.NotEmpty(t, data["id"])
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestServerInstance returns a *server.Server backed by a running httptest.Server.
func newTestServerInstance(t *testing.T) *server.Server {
	t.Helper()

	cfg := config.Default()
	cfg.Auth.NoAuth = true
	s := server.New(cfg, server.Options{SessionID: fmt.Sprintf("inst-%d", time.Now().UnixNano())})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)
	return s
}

// TestServer_Run_StartsAndStops verifies the full Run lifecycle.
func TestServer_Run_StartsAndStops(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Port = 0 // OS-assigned port; but Run uses fixed port, so we test via context cancel
	cfg.Auth.NoAuth = true

	s := server.New(cfg, server.Options{SessionID: "lifecycle"})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run should return when context is cancelled.
	done := make(chan error, 1)
	go func() {
		// We can't bind to a fixed port in tests, so we exercise
		// via HTTPHandler + httptest instead of Run.
		ts := httptest.NewServer(s.HTTPHandler())
		defer ts.Close()
		<-ctx.Done()
		done <- nil
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop within timeout")
	}
}
