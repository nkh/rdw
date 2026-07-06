package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/nkh/rdw/internal/auth"
	"github.com/nkh/rdw/internal/bindings"
	"github.com/nkh/rdw/internal/cycle"
	"github.com/nkh/rdw/internal/format"
	"github.com/nkh/rdw/internal/pipeline"
	"github.com/nkh/rdw/internal/highlight"
	"github.com/nkh/rdw/internal/terminal"
	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/layout"
	"github.com/nkh/rdw/internal/session"
)

// routes registers all HTTP handlers on mux.
func routes(mux *http.ServeMux, s *Server, cfg routeConfig) {
	rl := s.rl
	ts := s.tokenStore

	// Unauthenticated + rate-limited.
	unauth := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, rateLimitMiddleware(rl, h))
	}

	// Authenticated.
	authed := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, authMiddleware(ts, cfg.noAuth, http.HandlerFunc(h)))
	}

	// Admin (loopback + authenticated).
	admin := func(pattern string, h http.HandlerFunc) {
		inner := authMiddleware(ts, cfg.noAuth, http.HandlerFunc(h))
		if cfg.adminLocalOnly {
			inner = loopbackOnly(inner)
		}
		mux.Handle(pattern, inner)
	}

	// Health probe — unauthenticated, rate-limited.
	unauth("GET /api/v1/ping", handlePing)

	// WebSocket — auth checked inside.
	mux.HandleFunc("GET /api/v1/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWS(s.hub, s.tokenStore, cfg.noAuth, w, r)
	})

	// Session info.
	authed("GET /api/v1/session", s.handleSession)

	// Bindings config (used by browser UI to set up keyboard shortcuts).
	unauth("GET /api/v1/bindings", http.HandlerFunc(s.handleBindings))

	// Stream ingest (POST line).
	authed("POST /api/v1/stream/{id}", s.handleStreamPost)

	// Windows.
	authed("GET /api/v1/windows", s.handleWindowList)
	authed("POST /api/v1/windows", s.handleWindowCreate)
	authed("DELETE /api/v1/windows/{name}", s.handleWindowClose)
	authed("PATCH /api/v1/windows/{name}", s.handleWindowRename)
	authed("POST /api/v1/windows/{name}/focus", s.handleWindowFocus)

	// Panes.
	authed("POST /api/v1/panes/{id}/split", s.handlePaneSplit)
	authed("POST /api/v1/panes/{id}/zoom", s.handlePaneZoom)
	authed("DELETE /api/v1/panes/{id}", s.handlePaneClose)
	authed("POST /api/v1/panes/{id}/resize", s.handlePaneResize)

	// Layouts.
	authed("GET /api/v1/layouts", s.handleLayoutList)
	authed("POST /api/v1/layouts", s.handleLayoutSave)
	authed("POST /api/v1/layouts/{name}/apply", s.handleLayoutApply)

	// KV store.
	authed("GET /api/v1/kv", s.handleKVList)
	authed("GET /api/v1/kv/{key}", s.handleKVGet)
	authed("PUT /api/v1/kv/{key}", s.handleKVSet)
	authed("DELETE /api/v1/kv/{key}", s.handleKVDelete)

	// Tokens.
	authed("GET /api/v1/tokens", s.handleTokenList)
	authed("POST /api/v1/tokens", s.handleTokenCreate)
	authed("DELETE /api/v1/tokens/{id}", s.handleTokenRevoke)

	// Groups.
	authed("POST /api/v1/groups/{name}/hide",  s.handleGroupAction("hide"))
	authed("POST /api/v1/groups/{name}/show",  s.handleGroupAction("show"))
	authed("POST /api/v1/groups/{name}/focus", s.handleGroupAction("focus"))
	authed("POST /api/v1/groups/{name}/kill",  s.handleGroupAction("kill"))

	// Pane swap.
	authed("POST /api/v1/panes/{id}/swap",   s.handlePaneSwap)
	authed("PATCH /api/v1/panes/{id}",        s.handlePaneRename)
	unauth("GET /api/v1/formatters",                   http.HandlerFunc(s.handleFormatters))
	authed("POST /api/v1/formatters",                  s.handleFormatterRegister)
	authed("DELETE /api/v1/formatters/{name}",         s.handleFormatterUnregister)
	authed("POST /api/v1/panes/{id}/format",  s.handlePaneFormat)
	authed("POST /api/v1/panes/{id}/filters", s.handlePaneFilterAdd)
	authed("GET /api/v1/panes/{id}/bookmarks",        s.handleBookmarkList)
	authed("PUT /api/v1/panes/{id}/bookmarks/{name}", s.handleBookmarkAdd)
	authed("DELETE /api/v1/panes/{id}/bookmarks/{name}", s.handleBookmarkDelete)
	authed("GET /api/v1/highlights",              s.handleHighlightList)
	authed("PUT /api/v1/highlights/{name}",       s.handleHighlightAdd)
	authed("DELETE /api/v1/highlights/{name}",    s.handleHighlightDelete)
	authed("POST /api/v1/panes/{id}/terminal",    s.handleTerminalLaunch)
	authed("DELETE /api/v1/panes/{id}/terminal",  s.handleTerminalKill)
	authed("POST /api/v1/cycle/start",            s.handleCycleStart)
	authed("POST /api/v1/cycle/stop",             s.handleCycleStop)
	authed("GET /api/v1/cycle/status",           s.handleCycleStatus)
	authed("GET /api/v1/status",                 s.handleStatus)
	authed("GET /api/v1/status/panes/{id}",      s.handleStatusPane)

	// Export (Markdown bundle).
	authed("POST /api/v1/export/pane",   s.handleExportPane)
	authed("POST /api/v1/export/window", s.handleExportWindow)
	authed("POST /api/v1/export/all",    s.handleExportAll)

	// Admin console.
	// Admin page — guarded by separate admin token (loopback + admin token).
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		if cfg.adminToken != "" {
			tok := r.Header.Get("Authorization")
			if tok != "Bearer "+cfg.adminToken {
				// Also accept ?token= query param for browser navigation.
				if r.URL.Query().Get("token") != cfg.adminToken {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}
		s.handleAdminPage(w, r)
	})

	admin("GET /api/v1/admin/connections", s.handleAdminConnections)

	// Embedded frontend.
	mux.HandleFunc("/", handleFrontend)
}

// routeConfig holds per-route configuration.
type routeConfig struct {
	noAuth         bool
	adminLocalOnly bool
	adminToken     string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func handlePing(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func handleWS(hub *Hub, ts *auth.Store, noAuth bool, w http.ResponseWriter, r *http.Request) {
	var tokenID string
	if !noAuth {
		header := r.Header.Get("Authorization")
		if len(header) < 8 {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		plain := header[7:]
		tok, ok := ts.Verify(plain)
		if !ok {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		tokenID = tok.ID
	}
	serveWS(hub, w, r, tokenID)
}

func handleFrontend(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(frontendHTML)
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

// handleBindings returns the server's keyboard binding map as JSON.
// Used by the browser UI on startup to configure its key dispatch table.
func (s *Server) handleBindings(w http.ResponseWriter, _ *http.Request) {
	b := bindings.Default()
	if s.cfg.Bindings != nil {
		overrides := make(bindings.Bindings, len(s.cfg.Bindings))
		for action, keys := range s.cfg.Bindings {
			overrides[bindings.Action(action)] = bindings.Binding{Keys: keys}
		}
		b = bindings.Merge(b, overrides)
	}
	jsonResponse(w, bindings.JSON(b))
}

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request) {
	snap, err := s.manager.Snapshot()
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(snap)
}

// ---------------------------------------------------------------------------
// Stream ingest
// ---------------------------------------------------------------------------

func (s *Server) handleStreamPost(w http.ResponseWriter, r *http.Request) {
	targetStr := r.PathValue("id")
	id, err := session.ParseTargetID(targetStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid target ID: "+err.Error())
		return
	}

	var body struct {
		Line string `json:"line"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := s.router.Route(id, body.Line); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Windows
// ---------------------------------------------------------------------------

func (s *Server) handleWindowList(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, map[string]interface{}{"windows": s.manager.Windows()})
}

func (s *Server) handleWindowCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.manager.CreateWindow(body.Name); err != nil {
		apiError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleWindowClose(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.manager.CloseWindow(name); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWindowRename(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.manager.RenameWindow(oldName, body.Name); err != nil {
		apiError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWindowFocus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.manager.FocusWindow(name); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	// Broadcast focus change to all connected browsers.
	data, _ := json.Marshal(SpecialMessage{
		Type:    "window_focus",
		Payload: map[string]string{"name": name},
	})
	s.hub.Broadcast(data)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Panes
// ---------------------------------------------------------------------------

func (s *Server) handlePaneSplit(w http.ResponseWriter, r *http.Request) {
	parentID := r.PathValue("id")
	var body struct {
		Direction string `json:"direction"` // "h" or "v"
		NewID     string `json:"new_id"`
		Group     string `json:"group,omitempty"`
		Private   bool   `json:"private,omitempty"`
		Size      string `json:"size,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Direction != "h" && body.Direction != "v" {
		apiError(w, http.StatusBadRequest, `direction must be "h" or "v"`)
		return
	}

	newID, err := session.ParseTargetID(body.NewID)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid new_id: "+err.Error())
		return
	}

	// Find the parent pane's window.
	parentTargetID, parseErr := session.ParseTargetID(parentID)
	if parseErr != nil {
		apiError(w, http.StatusBadRequest, "invalid parent target ID")
		return
	}

	win, _ := s.manager.FindPane(parentTargetID)
	if win == nil {
		apiError(w, http.StatusNotFound, "parent pane not found")
		return
	}

	pane := &session.PaneState{
		TargetID: newID,
		Group:    body.Group,
		Private:  body.Private,
		Split:    body.Direction,
		Size:     body.Size,
	}

	if err := s.manager.AddPane(win.Name, pane); err != nil {
		apiError(w, http.StatusConflict, err.Error())
		return
	}

	// Register the new pipeline in the router.
	if _, err := s.router.Register(newID, 0); err != nil {
		// Undo the pane addition.
		_ = s.manager.RemovePane(newID)
		apiError(w, http.StatusConflict, err.Error())
		return
	}

	s.broadcastLayoutUpdate()
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handlePaneZoom(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := session.ParseTargetID(idStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, p := s.manager.FindPane(id)
	if p == nil {
		apiError(w, http.StatusNotFound, "pane not found")
		return
	}
	data, _ := json.Marshal(SpecialMessage{
		Type:    "pane_zoom",
		Payload: map[string]string{"target_id": idStr},
	})
	s.hub.Broadcast(data)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePaneClose(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := session.ParseTargetID(idStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.manager.RemovePane(id); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	s.router.Deregister(id)
	s.broadcastLayoutUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePaneResize(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var body struct {
		Direction string `json:"direction"`
		Size      string `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, _, err := layout.ParseResizeArg(body.Size); err != nil {
		apiError(w, http.StatusBadRequest, "invalid size: "+err.Error())
		return
	}
	data, _ := json.Marshal(SpecialMessage{
		Type: "pane_resize",
		Payload: map[string]string{
			"target_id": idStr,
			"direction": body.Direction,
			"size":      body.Size,
		},
	})
	s.hub.Broadcast(data)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Layouts
// ---------------------------------------------------------------------------

type savedLayout struct {
	Name   string       `json:"name"`
	Layout layout.Layout `json:"layout"`
}

func (s *Server) handleLayoutList(w http.ResponseWriter, _ *http.Request) {
	s.layoutMu.RLock()
	defer s.layoutMu.RUnlock()

	names := make([]string, 0, len(s.layouts))
	for name := range s.layouts {
		names = append(names, name)
	}
	jsonResponse(w, map[string]interface{}{"layouts": names})
}

func (s *Server) handleLayoutSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		apiError(w, http.StatusBadRequest, "name required")
		return
	}

	snap, err := s.manager.Snapshot()
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.layoutMu.Lock()
	s.layouts[body.Name] = snap
	s.layoutMu.Unlock()

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleLayoutApply(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	s.layoutMu.RLock()
	snap, exists := s.layouts[name]
	s.layoutMu.RUnlock()

	if !exists {
		apiError(w, http.StatusNotFound, "layout not found: "+name)
		return
	}

	// Collect pane IDs before restore so we can deregister removed panes.
	before := s.manager.AllPaneIDs()

	if err := s.manager.RestoreSnapshot(snap) ; err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	after := make(map[session.TargetID]struct{})
	for _, id := range s.manager.AllPaneIDs() {
		after[id] = struct{}{}
	}

	// Deregister pipelines for panes no longer in the session.
	for _, id := range before {
		if _, kept := after[id] ; !kept {
			s.router.Deregister(id)
		}
	}

	// Register pipelines for panes that are new to the session.
	for id := range after {
		if _, ok := s.router.Get(id) ; !ok {
			_, _ = s.router.Register(id, s.cfg.Server.ScrollbackCap)
		}
	}

	s.broadcastLayoutUpdate()
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// KV store
// ---------------------------------------------------------------------------

func (s *Server) handleKVList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	keys := s.kv.Keys(prefix)

	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.String()
	}
	jsonResponse(w, map[string]interface{}{"keys": names})
}

func (s *Server) handleKVGet(w http.ResponseWriter, r *http.Request) {
	keyStr := r.PathValue("key")
	k, err := kvstore.ParseKey(keyStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid key: "+err.Error())
		return
	}

	v, ok := s.kv.Get(k)
	if !ok {
		apiError(w, http.StatusNotFound, "key not found")
		return
	}

	jsonResponse(w, map[string]string{"key": keyStr, "value": v})
}

func (s *Server) handleKVSet(w http.ResponseWriter, r *http.Request) {
	keyStr := r.PathValue("key")
	k, err := kvstore.ParseKey(keyStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid key: "+err.Error())
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := s.kv.Set(k, body.Value); err != nil {
		apiError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}

	if s.kvDB != nil {
		if err := s.kvDB.Persist(k, body.Value); err != nil {
			fmt.Fprintf(os.Stderr, "rdw: kv persist: %v\n", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleKVDelete(w http.ResponseWriter, r *http.Request) {
	keyStr := r.PathValue("key")
	k, err := kvstore.ParseKey(keyStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid key: "+err.Error())
		return
	}

	s.kv.Delete(k)

	if s.kvDB != nil {
		if err := s.kvDB.Remove(k); err != nil {
			fmt.Fprintf(os.Stderr, "rdw: kv remove: %v\n", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

func (s *Server) handleTokenList(w http.ResponseWriter, _ *http.Request) {
	tokens := s.tokenStore.List()
	type tokenView struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		ExpiresAt time.Time `json:"expires_at,omitempty"`
		Panes     []string  `json:"panes,omitempty"`
		Windows   []string  `json:"windows,omitempty"`
	}
	views := make([]tokenView, len(tokens))
	for i, t := range tokens {
		views[i] = tokenView{
			ID:        t.ID,
			CreatedAt: t.CreatedAt,
			ExpiresAt: t.ExpiresAt,
			Panes:     t.Panes,
			Windows:   t.Windows,
		}
	}
	jsonResponse(w, map[string]interface{}{"tokens": views})
}

func (s *Server) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpirySeconds int64    `json:"expiry_seconds"`
		Panes         []string `json:"panes,omitempty"`
		Windows       []string `json:"windows,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	expiry := time.Duration(body.ExpirySeconds) * time.Second
	if body.ExpirySeconds == 0 {
		expiry = 24 * time.Hour
	}

	plain, tok, err := s.tokenStore.Create(auth.CreateOptions{
		Expiry:  expiry,
		Panes:   body.Panes,
		Windows: body.Windows,
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonResponse(w, map[string]interface{}{
		"token":      plain,
		"id":         tok.ID,
		"expires_at": tok.ExpiresAt,
	})
}

func (s *Server) handleTokenRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.hub.RevokeToken(id)
	if !s.tokenStore.Revoke(id) {
		apiError(w, http.StatusNotFound, "token not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

func (s *Server) handleAdminConnections(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, map[string]int{"connections": s.hub.ConnCount()})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// broadcastLayoutUpdate sends a layout_update message to all browsers.
func (s *Server) broadcastLayoutUpdate() {
	snap, _ := s.manager.Snapshot()
	data, _ := json.Marshal(SpecialMessage{
		Type:    "layout_update",
		Payload: json.RawMessage(snap),
	})
	s.hub.Broadcast(data)
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

// handleGroupAction returns a handler that performs an action on all panes
// in a named group.
func (s *Server) handleGroupAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupName := r.PathValue("name")
		if groupName == "" {
			apiError(w, http.StatusBadRequest, "group name required")
			return
		}

		// Collect all panes belonging to this group.
		var targets []session.TargetID
		for _, win := range s.manager.Windows() {
			for _, p := range win.Panes {
				if p.Group == groupName {
					targets = append(targets, p.TargetID)
				}
			}
		}

		if len(targets) == 0 {
			apiError(w, http.StatusNotFound, "no panes found in group "+groupName)
			return
		}

		switch action {
		case "kill":
			for _, id := range targets {
				_ = s.manager.RemovePane(id)
				s.router.Deregister(id)
			}
			s.broadcastLayoutUpdate()

		case "hide", "show":
			// Broadcast visibility toggle to browsers.
			data, _ := json.Marshal(SpecialMessage{
				Type: "group_" + action,
				Payload: map[string]string{"group": groupName},
			})
			s.hub.Broadcast(data)

		case "focus":
			// Focus the first pane in the group.
			if len(targets) > 0 {
				data, _ := json.Marshal(SpecialMessage{
					Type: "pane_focus",
					Payload: map[string]string{"target_id": targets[0].String()},
				})
				s.hub.Broadcast(data)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ---------------------------------------------------------------------------
// Pane swap
// ---------------------------------------------------------------------------

func (s *Server) handlePaneSwap(w http.ResponseWriter, r *http.Request) {
	aStr := r.PathValue("id")
	var body struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	idA, err := session.ParseTargetID(aStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid pane ID: "+err.Error())
		return
	}
	idB, err := session.ParseTargetID(body.Target)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid target ID: "+err.Error())
		return
	}

	winA, paneA := s.manager.FindPane(idA)
	winB, paneB := s.manager.FindPane(idB)

	if paneA == nil {
		apiError(w, http.StatusNotFound, "pane "+aStr+" not found")
		return
	}
	if paneB == nil {
		apiError(w, http.StatusNotFound, "pane "+body.Target+" not found")
		return
	}

	// Swap grid positions by exchanging Split and Size fields.
	paneA.Split, paneB.Split = paneB.Split, paneA.Split
	paneA.Size,  paneB.Size  = paneB.Size,  paneA.Size

	_ = winA
	_ = winB

	s.broadcastLayoutUpdate()
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

func (s *Server) handleExportPane(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetID string `json:"target_id"`
		OutDir   string `json:"out_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	id, err := session.ParseTargetID(body.TargetID)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid target ID: "+err.Error())
		return
	}

	_, pane := s.manager.FindPane(id)
	if pane == nil {
		apiError(w, http.StatusNotFound, "pane not found: "+body.TargetID)
		return
	}

	outDir := body.OutDir
	if outDir == "" {
		outDir = "."
	}

	if err := s.exportPane(id, outDir); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExportWindow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		OutDir string `json:"out_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	win := s.manager.Window(body.Name)
	if win == nil {
		apiError(w, http.StatusNotFound, "window not found: "+body.Name)
		return
	}

	outDir := body.OutDir
	if outDir == "" {
		outDir = "."
	}

	if err := s.exportWindow(win, outDir); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExportAll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OutDir string `json:"out_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	outDir := body.OutDir
	if outDir == "" {
		outDir = "."
	}

	if err := s.exportAll(outDir); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePaneRename sets a pane's display title. Accepts {"title":"..."} or {"label":"..."}.
func (s *Server) handlePaneRename(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var body struct {
		Title string `json:"title"`
		Label string `json:"label"` // backward compat
	}
	if err := json.NewDecoder(r.Body).Decode(&body) ; err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	title := body.Title
	if title == "" {
		title = body.Label
	}
	if title == "" {
		apiError(w, http.StatusBadRequest, "title is required")
		return
	}

	id, err := session.ParseTargetID(idStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid pane ID: "+err.Error())
		return
	}

	_, pane := s.manager.FindPane(id)
	if pane == nil {
		apiError(w, http.StatusNotFound, "pane not found: "+idStr)
		return
	}

	pane.Label = title
	s.broadcastLayoutUpdate()
	w.WriteHeader(http.StatusNoContent)
}

// handleFormatters returns the list of available formatter names.
func (s *Server) handleFormatters(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, map[string][]string{"formatters": format.Names()})
}

// handlePaneFormat formats a pane's scrollback through the named formatter.
func (s *Server) handlePaneFormat(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var body struct {
		Formatter string `json:"formatter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	id, err := session.ParseTargetID(idStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid pane ID: "+err.Error())
		return
	}

	pl, ok := s.router.Get(id)
	if !ok {
		apiError(w, http.StatusNotFound, "pane pipeline not found: "+idStr)
		return
	}

	f, err := format.Get(body.Formatter)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	lines := pl.Scrollback().Lines()
	html, err := f.Format(lines)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, map[string]string{"html": html})
}

// handleBookmarkList returns all bookmarks for a pane, sorted by line index.
func (s *Server) handleBookmarkList(w http.ResponseWriter, r *http.Request) {
	id, err := session.ParseTargetID(r.PathValue("id"))
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	bs := s.router.Bookmarks(id)
	if bs == nil {
		apiError(w, http.StatusNotFound, "pane not found: "+r.PathValue("id"))
		return
	}

	jsonResponse(w, map[string]any{"bookmarks": bs.All()})
}

// handleBookmarkAdd creates or replaces a named bookmark on a pane.
func (s *Server) handleBookmarkAdd(w http.ResponseWriter, r *http.Request) {
	id, err := session.ParseTargetID(r.PathValue("id"))
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		LineIndex int `json:"line_index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	bs := s.router.Bookmarks(id)
	if bs == nil {
		apiError(w, http.StatusNotFound, "pane not found: "+r.PathValue("id"))
		return
	}

	name := r.PathValue("name")
	if err := bs.Add(name, body.LineIndex); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleBookmarkDelete removes a named bookmark from a pane.
func (s *Server) handleBookmarkDelete(w http.ResponseWriter, r *http.Request) {
	id, err := session.ParseTargetID(r.PathValue("id"))
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	bs := s.router.Bookmarks(id)
	if bs == nil {
		apiError(w, http.StatusNotFound, "pane not found: "+r.PathValue("id"))
		return
	}

	if err := bs.Remove(r.PathValue("name")); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleHighlightList returns all stored highlight profiles.
func (s *Server) handleHighlightList(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, map[string]any{"profiles": s.highlights.All()})
}

// handleHighlightAdd creates or replaces a named highlight profile.
func (s *Server) handleHighlightAdd(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		Rules []highlight.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	p := highlight.Profile{Name: name, Rules: body.Rules}
	if err := s.highlights.Add(p); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleHighlightDelete removes a named highlight profile.
func (s *Server) handleHighlightDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.highlights.Remove(r.PathValue("name")); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleTerminalLaunch starts a restricted terminal subprocess for a pane.
// The response includes the local port the terminal listens on.
func (s *Server) handleTerminalLaunch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Cmd string `json:"cmd"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	port, err := s.terminals.Launch(id, body.Cmd)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, map[string]any{
		"id":   id,
		"port": port,
		"url":  fmt.Sprintf("http://127.0.0.1:%d", port),
	})
}

// handleTerminalKill stops a running terminal pane subprocess.
func (s *Server) handleTerminalKill(w http.ResponseWriter, r *http.Request) {
	if err := s.terminals.Kill(r.PathValue("id")); err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// terminalForTest exposes the terminal manager for integration tests.
func (s *Server) terminalForTest() *terminal.Manager { return s.terminals }

// handleCycleStart begins focus-cycle rotation across the supplied window list.
func (s *Server) handleCycleStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Windows  []string `json:"windows"`
		Interval int      `json:"interval_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if body.Interval <= 0 {
		body.Interval = 5000
	}

	c, err := cycle.New(body.Windows, time.Duration(body.Interval)*time.Millisecond)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.cycleMu.Lock()
	if s.cycleCancel != nil {
		s.cycleCancel()
	}
	ctx, cancel := context.WithCancel(s.runCtx)
	s.cycleCancel = cancel
	s.cycleState.Windows = body.Windows
	s.cycleState.Interval = body.Interval
	s.cycleMu.Unlock()

	ch := c.Run(ctx)

	go func() {
		for ev := range ch {
			if err := s.manager.FocusWindow(ev.Window); err != nil {
				// Window missing — log and skip; cycle continues to next window.
				fmt.Fprintf(os.Stderr, "rdw: cycle: focus %q: %v\n", ev.Window, err)
				continue
			}
			s.broadcastLayoutUpdate()
		}
	}()

	jsonResponse(w, map[string]any{"windows": body.Windows, "interval_ms": body.Interval})
}

// handleCycleStop stops the running focus cycle, if any.
func (s *Server) handleCycleStop(w http.ResponseWriter, _ *http.Request) {
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	if s.cycleCancel == nil {
		apiError(w, http.StatusConflict, "no cycle running")
		return
	}

	s.cycleCancel()
	s.cycleCancel = nil
	w.WriteHeader(http.StatusNoContent)
}

// handlePaneFilterAdd attaches a shell-command filter to a pane's pipeline.
func (s *Server) handlePaneFilterAdd(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	var body struct {
		Cmd string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body) ; err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if body.Cmd == "" {
		apiError(w, http.StatusBadRequest, "cmd is required")
		return
	}

	id, err := session.ParseTargetID(idStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid pane ID: "+err.Error())
		return
	}

	pl, ok := s.router.Get(id)
	if !ok {
		apiError(w, http.StatusNotFound, "pane not found: "+idStr)
		return
	}

	cf, err := pipeline.NewCmdFilterWithKV(body.Cmd, s.kv)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := pl.AddFilter(cf.Filter) ; err != nil {
		_ = cf.Close()
		apiError(w, http.StatusConflict, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleCycleStatus returns whether a cycle is running and its configuration.
func (s *Server) handleCycleStatus(w http.ResponseWriter, _ *http.Request) {
	s.cycleMu.Lock()
	running := s.cycleCancel != nil
	wins := s.cycleState.Windows
	interval := s.cycleState.Interval
	s.cycleMu.Unlock()

	jsonResponse(w, map[string]any{
		"running":     running,
		"windows":     wins,
		"interval_ms": interval,
	})
}

// handleFormatterRegister registers a new user-defined external formatter.
func (s *Server) handleFormatterRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Cmd  string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body) ; err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Name == "" || body.Cmd == "" {
		apiError(w, http.StatusBadRequest, "name and cmd are required")
		return
	}

	snap := format.KVSnapshot{}
	for k, v := range s.kv.Snapshot() {
		snap[k.String()] = v
	}

	f := format.NewCmdFormatter(body.Name, body.Cmd, snap)
	if err := format.Register(f) ; err != nil {
		apiError(w, http.StatusConflict, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleFormatterUnregister removes a user-registered formatter.
func (s *Server) handleFormatterUnregister(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := format.Unregister(name) ; err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleStatus returns a full server introspection snapshot.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	snap, _ := s.manager.Snapshot()

	var snapObj map[string]any
	_ = json.Unmarshal(snap, &snapObj)

	s.cycleMu.Lock()
	cycleRunning := s.cycleCancel != nil
	cycleWins    := s.cycleState.Windows
	cycleMs      := s.cycleState.Interval
	s.cycleMu.Unlock()

	// Per-pane stats.
	paneStats := map[string]any{}
	for _, id := range s.manager.AllPaneIDs() {
		pl, ok := s.router.Get(id)
		if !ok {
			continue
		}

		_, pane := s.manager.FindPane(id)
		label := ""
		formatter := ""
		if pane != nil {
			label     = pane.Label
			formatter = pane.Formatter
		}

		bms := s.router.Bookmarks(id)
		bmCount := 0
		if bms != nil {
			bmCount = bms.Len()
		}

		paneStats[id.String()] = map[string]any{
			"target_id":       id.String(),
			"title":           label,
			"formatter":       formatter,
			"scrollback_len":  pl.Scrollback().Len(),
			"bookmarks":       bmCount,
		}
	}

	// Saved layouts.
	s.layoutMu.RLock()
	layouts := make([]string, 0, len(s.layouts))
	for name := range s.layouts {
		layouts = append(layouts, name)
	}
	s.layoutMu.RUnlock()

	// Active highlight profiles.
	hlProfiles := make([]string, 0)
	for _, p := range s.highlights.All() {
		hlProfiles = append(hlProfiles, p.Name)
	}

	// Active tokens.
	tokens := s.tokenStore.List()
	tokenInfos := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		tokenInfos = append(tokenInfos, map[string]any{
			"id":      t.ID,
			"panes":   t.Panes,
			"windows": t.Windows,
			"expires": t.ExpiresAt,
		})
	}

	// Active WebSocket connections.
	connCount := s.hub.ConnCount()

	// Formatters.
	formatNames := format.Names()

	jsonResponse(w, map[string]any{
		"port":         s.port,
		"session":      snapObj,
		"panes":        paneStats,
		"layouts":      layouts,
		"formatters":   formatNames,
		"highlights":   hlProfiles,
		"tokens":       tokenInfos,
		"connections":  connCount,
		"cycle": map[string]any{
			"running":     cycleRunning,
			"windows":     cycleWins,
			"interval_ms": cycleMs,
		},
		"kv_len": s.kv.Len(),
	})
}

// handleStatusPane returns detailed status for a single pane.
func (s *Server) handleStatusPane(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := session.ParseTargetID(idStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid pane ID: "+err.Error())
		return
	}

	pl, ok := s.router.Get(id)
	if !ok {
		apiError(w, http.StatusNotFound, "pane not found: "+idStr)
		return
	}

	_, pane := s.manager.FindPane(id)
	label     := ""
	formatter := ""
	savedFmt  := ""
	if pane != nil {
		label     = pane.Label
		formatter = pane.Formatter
		savedFmt  = pane.SavedFormatter
	}

	bms := s.router.Bookmarks(id)
	bmList := []any{}
	if bms != nil {
		for _, b := range bms.All() {
			bmList = append(bmList, map[string]any{
				"name":       b.Name,
				"line_index": b.LineIndex,
				"created_at": b.CreatedAt,
			})
		}
	}

	lines := pl.Scrollback().Lines()
	lastLine := ""
	if len(lines) > 0 {
		lastLine = lines[len(lines)-1]
	}

	jsonResponse(w, map[string]any{
		"target_id":       idStr,
		"title":           label,
		"formatter":       formatter,
		"saved_formatter": savedFmt,
		"scrollback_len":  pl.Scrollback().Len(),
		"scrollback_cap":  pl.Scrollback().Cap(),
		"last_line":       lastLine,
		"bookmarks":       bmList,
	})
}

// handleAdminPage serves the /admin introspection UI.
func (s *Server) handleAdminPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(adminHTML))
}

const adminHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>rdw admin</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font: 13px/1.5 monospace; background: #1a1a1a; color: #ccc; padding: 16px; }
h1 { color: #fff; margin-bottom: 16px; font-size: 16px; }
h2 { color: #aaa; margin: 16px 0 8px; font-size: 13px; text-transform: uppercase; letter-spacing: 1px; }
table { border-collapse: collapse; width: 100%; margin-bottom: 16px; }
th { text-align: left; color: #888; font-weight: normal; padding: 2px 12px 2px 0; border-bottom: 1px solid #333; }
td { padding: 2px 12px 2px 0; }
tr:hover td { background: #222; }
.ok  { color: #5c5; }
.off { color: #888; }
.err { color: #c55; }
#reload { float: right; font: 12px monospace; background: #333; color: #ccc; border: 1px solid #555;
          padding: 2px 8px; cursor: pointer; }
#reload:hover { background: #444; }
</style>
</head>
<body>
<h1>rdw admin <button id="reload" onclick="load()">refresh</button></h1>
<div id="content">loading...</div>
<script>
function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
}

function row(k, v, cls) {
  return '<tr><td>' + esc(k) + '</td><td class="' + (cls||'') + '">' + esc(v) + '</td></tr>'
}

function table(title, rows) {
  if (!rows.length) return ''
  var html = '<h2>' + esc(title) + '</h2><table>'
  html += rows.join('')
  return html + '</table>'
}

function load() {
  var token = new URLSearchParams(location.search).get('token') || ''
  var headers = token ? {'Authorization': 'Bearer ' + token} : {}
  fetch('/api/v1/status', {headers: headers})
    .then(function(r) { return r.json() })
    .then(function(d) { render(d) })
    .catch(function(e) { document.getElementById('content').textContent = 'Error: ' + e })
}

function render(d) {
  var html = ''

  // Server
  html += table('Server', [
    row('port', d.port),
    row('connections', d.connections),
    row('kv_entries', d.kv_len),
  ])

  // Cycle
  if (d.cycle) {
    var c = d.cycle
    html += table('Focus cycle', [
      row('running', c.running, c.running ? 'ok' : 'off'),
      c.running ? row('windows', (c.windows||[]).join(', ')) : '',
      c.running ? row('interval_ms', c.interval_ms) : '',
    ])
  }

  // Panes
  if (d.panes) {
    var paneRows = []
    for (var id in d.panes) {
      var p = d.panes[id]
      paneRows.push(row(id,
        'title=' + (p.title||'—') +
        '  scrollback=' + p.scrollback_len +
        '  formatter=' + (p.formatter||'text') +
        '  bookmarks=' + p.bookmarks
      ))
    }
    html += table('Panes (' + paneRows.length + ')', paneRows)
  }

  // Formatters
  if (d.formatters) {
    html += table('Formatters', d.formatters.map(function(f) { return row(f, '') }))
  }

  // Highlights
  if (d.highlights && d.highlights.length) {
    html += table('Highlight profiles', d.highlights.map(function(h) { return row(h, '') }))
  }

  // Layouts
  if (d.layouts && d.layouts.length) {
    html += table('Saved layouts', d.layouts.map(function(l) { return row(l, '') }))
  }

  // Tokens
  if (d.tokens) {
    var trows = d.tokens.map(function(t) {
      return row(t.id, 'panes=' + JSON.stringify(t.panes) + ' expires=' + t.expires)
    })
    html += table('Tokens (' + trows.length + ')', trows)
  }

  document.getElementById('content').innerHTML = html || '<p>No data</p>'
}

load()
setInterval(load, 10000)
</script>
</body>
</html>`
