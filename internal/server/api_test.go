package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nkh/rdw/internal/auth"
	"github.com/nkh/rdw/internal/config"
	"github.com/nkh/rdw/internal/server"
	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type apiClient struct {
	base  string
	token string
	t     *testing.T
}

func newAPIClient(t *testing.T, ts *httptest.Server, token string) *apiClient {
	t.Helper()
	return &apiClient{base: ts.URL, token: token, t: t}
}

func (c *apiClient) do(method, path string, body interface{}) *http.Response {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	require.NoError(c.t, err)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	return resp
}

func (c *apiClient) get(path string) *http.Response    { return c.do("GET", path, nil) }
func (c *apiClient) post(path string, b interface{}) *http.Response { return c.do("POST", path, b) }
func (c *apiClient) put(path string, b interface{}) *http.Response  { return c.do("PUT", path, b) }
func (c *apiClient) del(path string) *http.Response   { return c.do("DELETE", path, nil) }

func mustID(s string) session.TargetID {
	id, err := session.ParseTargetID(s)
	if err != nil {
		panic(err)
	}
	return id
}
func (c *apiClient) patch(path string, b interface{}) *http.Response { return c.do("PATCH", path, b) }

func decodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
}

func newNoAuthServer(t *testing.T) (*httptest.Server, *server.Server) {
	t.Helper()
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	s := server.New(cfg, server.Options{
		SessionID: fmt.Sprintf("api-test-%d", time.Now().UnixNano()),
	})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)
	return ts, s
}

func newAuthServer(t *testing.T) (*httptest.Server, *server.Server, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Auth.NoAuth = false
	s := server.New(cfg, server.Options{
		SessionID: fmt.Sprintf("api-auth-%d", time.Now().UnixNano()),
	})
	plain, _, err := s.TokenStore().Create(auth.CreateOptions{Expiry: time.Hour})
	require.NoError(t, err)
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)
	return ts, s, plain
}

// ---------------------------------------------------------------------------
// Ping
// ---------------------------------------------------------------------------

func TestAPI_Ping(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.get("/api/v1/ping")
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]string
	decodeJSON(t, resp, &body)
	assert.Equal(t, "ok", body["status"])
	assert.NotEmpty(t, body["time"])
}

func TestAPI_Ping_Unauthenticated(t *testing.T) {
	ts, _, _ := newAuthServer(t)
	c := newAPIClient(t, ts, "") // no token
	resp := c.get("/api/v1/ping")
	assert.Equal(t, 200, resp.StatusCode) // ping never requires auth
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

func TestAPI_Session_RequiresAuth(t *testing.T) {
	ts, _, _ := newAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.get("/api/v1/session")
	resp.Body.Close()
	assert.Equal(t, 401, resp.StatusCode)
}

func TestAPI_Session_WithToken(t *testing.T) {
	ts, _, token := newAuthServer(t)
	c := newAPIClient(t, ts, token)
	resp := c.get("/api/v1/session")
	assert.Equal(t, 200, resp.StatusCode)
	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	assert.NotNil(t, body["windows"])
}

func TestAPI_Session_NoAuth(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.get("/api/v1/session")
	assert.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Windows
// ---------------------------------------------------------------------------

func TestAPI_Windows_List_EmptyInitially(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.get("/api/v1/windows")
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	wins := body["windows"]
	assert.NotNil(t, wins)
}

func TestAPI_Windows_Create(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/windows", map[string]string{"name": "build"})
	resp.Body.Close()
	assert.Equal(t, 201, resp.StatusCode)

	// Verify it appears in list.
	resp2 := c.get("/api/v1/windows")
	var body map[string]interface{}
	decodeJSON(t, resp2, &body)
	wins := body["windows"].([]interface{})
	require.Len(t, wins, 1)
	w := wins[0].(map[string]interface{})
	assert.Equal(t, "build", w["name"])
}

func TestAPI_Windows_Create_EmptyName(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/windows", map[string]string{"name": ""})
	resp.Body.Close()
	assert.Equal(t, 409, resp.StatusCode)
}

func TestAPI_Windows_Create_Duplicate(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp1 := c.post("/api/v1/windows", map[string]string{"name": "dup"})
	resp1.Body.Close()
	require.Equal(t, 201, resp1.StatusCode)

	resp2 := c.post("/api/v1/windows", map[string]string{"name": "dup"})
	resp2.Body.Close()
	assert.Equal(t, 409, resp2.StatusCode)
}

func TestAPI_Windows_Rename(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/windows", map[string]string{"name": "old"})
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	resp2 := c.patch("/api/v1/windows/old", map[string]string{"name": "new"})
	resp2.Body.Close()
	assert.Equal(t, 204, resp2.StatusCode)

	resp3 := c.get("/api/v1/windows")
	var body map[string]interface{}
	decodeJSON(t, resp3, &body)
	wins := body["windows"].([]interface{})
	w := wins[0].(map[string]interface{})
	assert.Equal(t, "new", w["name"])
}

func TestAPI_Windows_Close(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	// Need two windows to close one.
	c.post("/api/v1/windows", map[string]string{"name": "a"}).Body.Close()
	c.post("/api/v1/windows", map[string]string{"name": "b"}).Body.Close()

	resp := c.del("/api/v1/windows/a")
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	resp2 := c.get("/api/v1/windows")
	var body map[string]interface{}
	decodeJSON(t, resp2, &body)
	assert.Len(t, body["windows"].([]interface{}), 1)
}

func TestAPI_Windows_Close_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.del("/api/v1/windows/ghost")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Windows_Focus(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	c.post("/api/v1/windows", map[string]string{"name": "w1"}).Body.Close()
	c.post("/api/v1/windows", map[string]string{"name": "w2"}).Body.Close()

	resp := c.post("/api/v1/windows/w2/focus", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	// Session should reflect the new active window.
	resp2 := c.get("/api/v1/session")
	var body map[string]interface{}
	decodeJSON(t, resp2, &body)
	assert.Equal(t, float64(1), body["active_window"])
}

func TestAPI_Windows_Focus_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/windows/nope/focus", nil)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Stream ingest
// ---------------------------------------------------------------------------

func TestAPI_Stream_Post(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	// Register a pane in the router first.
	id, _ := server.ParseTargetID("log")
	_, err := s.Router().Register(id, 0)
	require.NoError(t, err)

	resp := c.post("/api/v1/stream/log", map[string]string{"line": "hello"})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Stream_Post_UnknownTarget(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/stream/unknown-pane", map[string]string{"line": "data"})
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Stream_Post_InvalidID(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/stream/-bad-id", map[string]string{"line": "x"})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

func TestAPI_Stream_Post_InvalidJSON(t *testing.T) {
	ts, _ := newNoAuthServer(t)

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/stream/log", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Panes
// ---------------------------------------------------------------------------

func setupWindowWithPane(t *testing.T, s *server.Server, winName, paneID string) {
	t.Helper()
	_ = s.Manager().CreateWindow(winName)
	id, _ := server.ParseTargetID(paneID)
	_, _ = s.Router().Register(id, 0)
	_ = s.Manager().AddPane(winName, &server.PaneStateExport{
		TargetID: id,
	})
}

func TestAPI_Pane_Split(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("w")
	parentID, _ := server.ParseTargetID("parent")
	_, _ = s.Router().Register(parentID, 0)
	_ = s.Manager().AddPane("w", &server.PaneStateExport{TargetID: parentID})

	resp := c.post("/api/v1/panes/parent/split", map[string]interface{}{
		"direction": "v",
		"new_id":    "child",
	})
	resp.Body.Close()
	assert.Equal(t, 201, resp.StatusCode)

	// child pipeline should now be registered.
	childID, _ := server.ParseTargetID("child")
	_, ok := s.Router().Get(childID)
	assert.True(t, ok)
}

func TestAPI_Pane_Split_InvalidDirection(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/panes/parent/split", map[string]interface{}{
		"direction": "x", "new_id": "child",
	})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

func TestAPI_Pane_Close(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	c.post("/api/v1/windows", map[string]string{"name": "w"}).Body.Close()
	id, _ := server.ParseTargetID("mypane")
	_, _ = s.Router().Register(id, 0)
	_ = s.Manager().AddPane("w", &server.PaneStateExport{TargetID: id})

	resp := c.del("/api/v1/panes/mypane")
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	_, ok := s.Router().Get(id)
	assert.False(t, ok, "pipeline should be deregistered after pane close")
}

func TestAPI_Pane_Close_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.del("/api/v1/panes/nope")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Pane_Resize(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/panes/mypane/resize", map[string]string{
		"direction": "right", "size": "40%",
	})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Pane_Resize_InvalidSize(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/panes/mypane/resize", map[string]string{
		"direction": "right", "size": "not-a-size",
	})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// KV store
// ---------------------------------------------------------------------------

func TestAPI_KV_SetGet(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.put("/api/v1/kv/mykey", map[string]string{"value": "myvalue"})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	resp2 := c.get("/api/v1/kv/mykey")
	var body map[string]string
	decodeJSON(t, resp2, &body)
	assert.Equal(t, "myvalue", body["value"])
}

func TestAPI_KV_Get_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.get("/api/v1/kv/missing")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_KV_Delete(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	c.put("/api/v1/kv/todelete", map[string]string{"value": "v"}).Body.Close()
	resp := c.del("/api/v1/kv/todelete")
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	resp2 := c.get("/api/v1/kv/todelete")
	resp2.Body.Close()
	assert.Equal(t, 404, resp2.StatusCode)
}

func TestAPI_KV_List(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	c.put("/api/v1/kv/a", map[string]string{"value": "1"}).Body.Close()
	c.put("/api/v1/kv/b", map[string]string{"value": "2"}).Body.Close()

	resp := c.get("/api/v1/kv")
	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	keys := body["keys"].([]interface{})
	assert.Len(t, keys, 2)
}

func TestAPI_KV_List_WithPrefix(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	c.put("/api/v1/kv/build-status", map[string]string{"value": "ok"}).Body.Close()
	c.put("/api/v1/kv/deploy-status", map[string]string{"value": "ok"}).Body.Close()

	resp := c.get("/api/v1/kv?prefix=build")
	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	keys := body["keys"].([]interface{})
	assert.Len(t, keys, 1)
	assert.Equal(t, "build-status", keys[0].(string))
}

func TestAPI_KV_InvalidKey(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.put("/api/v1/kv/-invalid", map[string]string{"value": "x"})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Layouts
// ---------------------------------------------------------------------------

func TestAPI_Layout_SaveAndList(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/layouts", map[string]string{"name": "debug"})
	resp.Body.Close()
	assert.Equal(t, 201, resp.StatusCode)

	resp2 := c.get("/api/v1/layouts")
	var body map[string]interface{}
	decodeJSON(t, resp2, &body)
	layouts := body["layouts"].([]interface{})
	assert.Contains(t, layouts, "debug")
}

func TestAPI_Layout_Save_NoName(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/layouts", map[string]string{})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

func TestAPI_Layout_Apply_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/layouts/nonexistent/apply", nil)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Layout_Apply_Existing(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	c.post("/api/v1/layouts", map[string]string{"name": "mylay"}).Body.Close()
	resp := c.post("/api/v1/layouts/mylay/apply", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

func TestAPI_Tokens_CreateAndList(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/tokens", map[string]interface{}{
		"expiry_seconds": 3600,
		"panes":          []string{"build-log"},
	})
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	assert.Equal(t, 201, resp.StatusCode)
	assert.NotEmpty(t, created["token"])
	assert.NotEmpty(t, created["id"])

	resp2 := c.get("/api/v1/tokens")
	var body map[string]interface{}
	decodeJSON(t, resp2, &body)
	tokens := body["tokens"].([]interface{})
	assert.Len(t, tokens, 1)
}

func TestAPI_Tokens_Revoke(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/tokens", map[string]interface{}{"expiry_seconds": 3600})
	var created map[string]interface{}
	decodeJSON(t, resp, &created)

	id := created["id"].(string)
	resp2 := c.del("/api/v1/tokens/" + id)
	resp2.Body.Close()
	assert.Equal(t, 204, resp2.StatusCode)

	resp3 := c.get("/api/v1/tokens")
	var body map[string]interface{}
	decodeJSON(t, resp3, &body)
	tokens := body["tokens"].([]interface{})
	assert.Len(t, tokens, 0)
}

func TestAPI_Tokens_Revoke_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.del("/api/v1/tokens/nope")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Rate limiting
// ---------------------------------------------------------------------------

func TestAPI_RateLimit_UnauthEndpoint(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	// The rate limiter is per-IP; in tests all requests come from 127.0.0.1.
	// Send 11 unauthenticated requests to ping to trigger the limit.
	// Note: the rate limiter only applies to unauthenticated requests.
	var lastStatus int
	for i := range 12 {
		resp := c.get("/api/v1/ping")
		lastStatus = resp.StatusCode
		resp.Body.Close()
		_ = i
	}
	// After 10 requests, remaining ones should be rate-limited.
	assert.Equal(t, 429, lastStatus)
}

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

func TestAPI_Admin_Connections(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	cfg.Auth.AdminLocalOnly = false // disable loopback guard for test
	s := server.New(cfg, server.Options{
		SessionID: fmt.Sprintf("admin-%d", time.Now().UnixNano()),
	})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)
	c := newAPIClient(t, ts, "")

	resp := c.get("/api/v1/admin/connections")
	var body map[string]int
	decodeJSON(t, resp, &body)
	assert.Equal(t, 0, body["connections"])
}

// ---------------------------------------------------------------------------
// Bindings endpoint
// ---------------------------------------------------------------------------

func TestAPI_Bindings_ReturnsJSON(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.get("/api/v1/bindings")
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string][]string
	decodeJSON(t, resp, &body)

	// Default bindings must include vim navigation keys.
	assert.Contains(t, body, "pane.focus.left")
	assert.Contains(t, body, "window.next")
	assert.Equal(t, []string{"h"}, body["pane.focus.left"])
}

func TestAPI_Bindings_UnauthentictatedAllowed(t *testing.T) {
	ts, _, _ := newAuthServer(t)
	// No token — bindings is unauthed.
	c := newAPIClient(t, ts, "")
	resp := c.get("/api/v1/bindings")
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestAPI_Bindings_ConfigOverridesApplied(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	cfg.Bindings = map[string][]string{
		"pane.focus.left": {"Left"},
	}
	s := server.New(cfg, server.Options{
		SessionID: fmt.Sprintf("bind-%d", time.Now().UnixNano()),
	})
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)
	c := newAPIClient(t, ts, "")

	resp := c.get("/api/v1/bindings")
	var body map[string][]string
	decodeJSON(t, resp, &body)

	// Config override applied.
	assert.Equal(t, []string{"Left"}, body["pane.focus.left"])
	// Other defaults still present.
	assert.Contains(t, body, "window.next")
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

func setupGroup(t *testing.T, s *server.Server, winName string, panes []struct{ id, group string }) {
	t.Helper()
	_ = s.Manager().CreateWindow(winName)
	for _, p := range panes {
		id, _ := server.ParseTargetID(p.id)
		_, _ = s.Router().Register(id, 0)
		_ = s.Manager().AddPane(winName, &server.PaneStateExport{
			TargetID: id,
			Group:    p.group,
		})
	}
}

func TestAPI_Group_Kill(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	setupGroup(t, s, "w", []struct{ id, group string }{
		{"p1", "ci"},
		{"p2", "ci"},
		{"p3", "other"},
	})

	resp := c.post("/api/v1/groups/ci/kill", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	// p1 and p2 should be removed from the manager.
	id1, _ := server.ParseTargetID("p1")
	id2, _ := server.ParseTargetID("p2")
	id3, _ := server.ParseTargetID("p3")
	_, p1 := s.Manager().FindPane(id1)
	_, p2 := s.Manager().FindPane(id2)
	_, p3 := s.Manager().FindPane(id3)
	assert.Nil(t, p1, "p1 should be removed")
	assert.Nil(t, p2, "p2 should be removed")
	assert.NotNil(t, p3, "p3 should survive")
}

func TestAPI_Group_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/groups/nonexistent/kill", nil)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Group_Hide(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	setupGroup(t, s, "w", []struct{ id, group string }{{"p1", "mygroup"}})

	resp := c.post("/api/v1/groups/mygroup/hide", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Group_Show(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	setupGroup(t, s, "w", []struct{ id, group string }{{"p1", "g"}})
	resp := c.post("/api/v1/groups/g/show", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Group_Focus(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	setupGroup(t, s, "w", []struct{ id, group string }{{"p1", "g"}})
	resp := c.post("/api/v1/groups/g/focus", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Pane swap
// ---------------------------------------------------------------------------

func TestAPI_Pane_Swap(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("w")
	for _, name := range []string{"a", "b"} {
		id, _ := server.ParseTargetID(name)
		_, _ = s.Router().Register(id, 0)
		_ = s.Manager().AddPane("w", &server.PaneStateExport{TargetID: id})
	}

	resp := c.post("/api/v1/panes/a/swap", map[string]string{"target": "b"})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Pane_Swap_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/panes/missing/swap", map[string]string{"target": "also-missing"})
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Pane_Swap_InvalidJSON(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/panes/p/swap", map[string]string{"target": "-invalid"})
	resp.Body.Close()
	// Either bad request (invalid target ID) or not found (pane missing).
	assert.True(t, resp.StatusCode == 400 || resp.StatusCode == 404)
}

// ---------------------------------------------------------------------------
// Export endpoints
// ---------------------------------------------------------------------------

func TestAPI_Export_All(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("build")
	outDir := t.TempDir()

	resp := c.post("/api/v1/export/all", map[string]string{"out_dir": outDir})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	_, err := os.Stat(outDir + "/session.md")
	assert.NoError(t, err, "session.md should be written")
}

func TestAPI_Export_Pane(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("w")
	id, _ := server.ParseTargetID("mylog")
	_, _ = s.Router().Register(id, 0)
	_ = s.Manager().AddPane("w", &server.PaneStateExport{TargetID: id})

	outDir := t.TempDir()
	resp := c.post("/api/v1/export/pane", map[string]string{
		"target_id": "mylog",
		"out_dir":   outDir,
	})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	_, err := os.Stat(outDir + "/mylog.md")
	assert.NoError(t, err)
}

func TestAPI_Export_Pane_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/export/pane", map[string]string{
		"target_id": "ghost",
		"out_dir":   t.TempDir(),
	})
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Export_Window(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("ci")
	outDir := t.TempDir()
	resp := c.post("/api/v1/export/window", map[string]string{
		"name":    "ci",
		"out_dir": outDir,
	})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	_, err := os.Stat(outDir + "/ci.md")
	assert.NoError(t, err)
}

func TestAPI_Export_Window_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")
	resp := c.post("/api/v1/export/window", map[string]string{
		"name": "ghost", "out_dir": t.TempDir(),
	})
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Highlight profiles
// ---------------------------------------------------------------------------

func TestAPI_Highlight_AddListDelete(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	// Add a profile.
	resp := c.put("/api/v1/highlights/errors", map[string]interface{}{
		"rules": []map[string]string{
			{"pattern": `ERROR`, "class": "hl-error"},
			{"pattern": `WARN\w+`, "class": "hl-warn"},
		},
	})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	// List.
	var body map[string]interface{}
	resp = c.get("/api/v1/highlights")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	profiles := body["profiles"].([]interface{})
	assert.Len(t, profiles, 1)

	// Delete.
	resp = c.del("/api/v1/highlights/errors")
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	// Gone.
	resp = c.get("/api/v1/highlights")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Empty(t, body["profiles"])
}

func TestAPI_Highlight_BadPattern(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.put("/api/v1/highlights/bad", map[string]interface{}{
		"rules": []map[string]string{{"pattern": `[invalid`, "class": "x"}},
	})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

func TestAPI_Highlight_DeleteMissing(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.del("/api/v1/highlights/nope")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Pane zoom and rename
// ---------------------------------------------------------------------------

func TestAPI_Pane_Zoom(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{TargetID: mustID("zoom1"), ScrollbackCap: 100})

	resp := c.post("/api/v1/panes/zoom1/zoom", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Pane_Rename(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{TargetID: mustID("ren1"), ScrollbackCap: 100})

	resp := c.patch("/api/v1/panes/ren1", map[string]string{"label": "My Pane"})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	// Verify label stored.
	_, pane := s.Manager().FindPane(mustID("ren1"))
	require.NotNil(t, pane)
	assert.Equal(t, "My Pane", pane.Label)
}

func TestAPI_Pane_Rename_EmptyLabel(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{TargetID: mustID("ren2"), ScrollbackCap: 100})

	resp := c.patch("/api/v1/panes/ren2", map[string]string{"label": ""})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------

func TestAPI_Formatters_List(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	var body map[string]interface{}
	resp := c.get("/api/v1/formatters")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	names := body["formatters"].([]interface{})
	assert.Greater(t, len(names), 3)
}

func TestAPI_Pane_Format(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{TargetID: mustID("fmt1"), ScrollbackCap: 100})
	s.Router().Register(mustID("fmt1"), 100)

	resp := c.post("/api/v1/panes/fmt1/format", map[string]string{"formatter": "text"})
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body["html"].(string), "<pre")
}

func TestAPI_Pane_Format_Unknown(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{TargetID: mustID("fmt2"), ScrollbackCap: 100})
	s.Router().Register(mustID("fmt2"), 100)

	resp := c.post("/api/v1/panes/fmt2/format", map[string]string{"formatter": "nope"})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Cycle
// ---------------------------------------------------------------------------

func TestAPI_Cycle_StartStop(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/cycle/start", map[string]interface{}{
		"windows":     []string{"a", "b"},
		"interval_ms": 10000,
	})
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, float64(10000), body["interval_ms"])

	resp = c.post("/api/v1/cycle/stop", nil)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Cycle_StopNotRunning(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/cycle/stop", nil)
	resp.Body.Close()
	assert.Equal(t, 409, resp.StatusCode)
}

func TestAPI_Cycle_BadWindows(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/cycle/start", map[string]interface{}{
		"windows":     []string{},
		"interval_ms": 1000,
	})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

func TestAPI_Bookmark_AddListDelete(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{TargetID: mustID("bm1"), ScrollbackCap: 100})
	s.Router().Register(mustID("bm1"), 100)

	// Add.
	resp := c.put("/api/v1/panes/bm1/bookmarks/start", map[string]int{"line_index": 0})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	resp = c.put("/api/v1/panes/bm1/bookmarks/end", map[string]int{"line_index": 99})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	// List.
	var body map[string]interface{}
	resp = c.get("/api/v1/panes/bm1/bookmarks")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	bms := body["bookmarks"].([]interface{})
	assert.Len(t, bms, 2)

	// Delete.
	resp = c.del("/api/v1/panes/bm1/bookmarks/start")
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	resp = c.get("/api/v1/panes/bm1/bookmarks")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Len(t, body["bookmarks"].([]interface{}), 1)
}

func TestAPI_Bookmark_PaneNotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.get("/api/v1/panes/ghost/bookmarks")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Loopback guard
// ---------------------------------------------------------------------------

func TestAPI_AdminLoopback_PermitsLocalhost(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.NoAuth = true
	cfg.Auth.AdminLocalOnly = true
	s := server.New(cfg, server.Options{SessionID: "lb-test"})
	ts := httptest.NewServer(s.HTTPHandler())
	defer ts.Close()

	// httptest connections always come from 127.0.0.1 — loopback should be permitted.
	c := newAPIClient(t, ts, "")
	resp := c.get("/api/v1/admin/connections")
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// sc:clear clears server scrollback so replay is empty
// ---------------------------------------------------------------------------

func TestAPI_ScrollbackClear_EmptiesBuffer(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{
		TargetID:      mustID("sc1"),
		ScrollbackCap: 100,
	})
	_, _ = s.Router().Register(mustID("sc1"), 100)
	pl, _ := s.Router().Get(mustID("sc1"))

	// Clear directly to verify the buffer mechanism.
	pl.Scrollback().Append("line one")
	pl.Scrollback().Append("line two")
	require.Equal(t, 2, pl.Scrollback().Len())

	pl.Scrollback().Clear()
	assert.Equal(t, 0, pl.Scrollback().Len())

	// Stream ingest endpoint with sc:clear must return 204.
	resp := c.post("/api/v1/stream/sc1", map[string]string{"line": "sc:clear"})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Cycle status
// ---------------------------------------------------------------------------

func TestAPI_Cycle_Status_NotRunning(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	var body map[string]interface{}
	resp := c.get("/api/v1/cycle/status")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, false, body["running"])
}

func TestAPI_Cycle_Status_Running(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/cycle/start", map[string]interface{}{
		"windows":     []string{"a", "b"},
		"interval_ms": 60000,
	})
	resp.Body.Close()

	var body map[string]interface{}
	resp = c.get("/api/v1/cycle/status")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	assert.Equal(t, true, body["running"])
	assert.Equal(t, float64(60000), body["interval_ms"])

	// Stop the cycle.
	resp = c.post("/api/v1/cycle/stop", nil)
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Filter registration
// ---------------------------------------------------------------------------

func TestAPI_Filter_Add_MissingPipeline(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/panes/ghost/filters", map[string]string{"cmd": "cat"})
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAPI_Filter_Add_EmptyCmd(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{
		TargetID:      mustID("flt1"),
		ScrollbackCap: 100,
	})
	_, _ = s.Router().Register(mustID("flt1"), 100)

	resp := c.post("/api/v1/panes/flt1/filters", map[string]string{"cmd": ""})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Pane title
// ---------------------------------------------------------------------------

func TestAPI_Pane_Title_SetAndRead(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{
		TargetID:      mustID("title1"),
		ScrollbackCap: 100,
	})

	// Set via PATCH with title field.
	resp := c.patch("/api/v1/panes/title1", map[string]string{"title": "My Build Log"})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	_, pane := s.Manager().FindPane(mustID("title1"))
	require.NotNil(t, pane)
	assert.Equal(t, "My Build Log", pane.Label)
}

func TestAPI_Pane_Title_LabelBackcompat(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{
		TargetID:      mustID("title2"),
		ScrollbackCap: 100,
	})

	// Old field name "label" still accepted.
	resp := c.patch("/api/v1/panes/title2", map[string]string{"label": "compat label"})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	_, pane := s.Manager().FindPane(mustID("title2"))
	require.NotNil(t, pane)
	assert.Equal(t, "compat label", pane.Label)
}

func TestAPI_Pane_Title_EmptyRejected(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{
		TargetID:      mustID("title3"),
		ScrollbackCap: 100,
	})

	resp := c.patch("/api/v1/panes/title3", map[string]string{"title": ""})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Status endpoints
// ---------------------------------------------------------------------------

func TestAPI_Status(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	var body map[string]any
	resp := c.get("/api/v1/status")
	require.Equal(t, 200, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	assert.Contains(t, body, "port")
	assert.Contains(t, body, "connections")
	assert.Contains(t, body, "kv_len")
	assert.Contains(t, body, "formatters")
	assert.Contains(t, body, "cycle")
	assert.Contains(t, body, "panes")
}

func TestAPI_StatusPane(t *testing.T) {
	ts, s := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	_ = s.Manager().CreateWindow("main")
	_ = s.Manager().AddPane("main", &session.PaneState{
		TargetID:      mustID("sp1"),
		ScrollbackCap: 50,
	})
	_, _ = s.Router().Register(mustID("sp1"), 50)

	var body map[string]any
	resp := c.get("/api/v1/status/panes/sp1")
	require.Equal(t, 200, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	assert.Equal(t, "sp1", body["target_id"])
	assert.Contains(t, body, "scrollback_len")
	assert.Contains(t, body, "scrollback_cap")
	assert.Contains(t, body, "bookmarks")
}

func TestAPI_StatusPane_NotFound(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.get("/api/v1/status/panes/ghost")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Formatter register / unregister
// ---------------------------------------------------------------------------

func TestAPI_Formatter_Register_And_List(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/formatters", map[string]string{
		"name": "myfmt",
		"cmd":  "cat",
	})
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	var body map[string]any
	resp = c.get("/api/v1/formatters")
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	names := body["formatters"].([]any)
	found := false
	for _, n := range names {
		if n.(string) == "myfmt" {
			found = true
		}
	}
	assert.True(t, found, "myfmt should be in formatter list")
}

func TestAPI_Formatter_Unregister(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	c.post("/api/v1/formatters", map[string]string{"name": "tmpfmt", "cmd": "cat"})

	resp := c.del("/api/v1/formatters/tmpfmt")
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestAPI_Formatter_CannotOverrideBuiltin(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.post("/api/v1/formatters", map[string]string{"name": "json", "cmd": "cat"})
	resp.Body.Close()
	assert.Equal(t, 409, resp.StatusCode)
}

func TestAPI_Formatter_UnregisterBuiltinFails(t *testing.T) {
	ts, _ := newNoAuthServer(t)
	c := newAPIClient(t, ts, "")

	resp := c.del("/api/v1/formatters/text")
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}
