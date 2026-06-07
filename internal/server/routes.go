package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nkh/rdw/internal/auth"
	"github.com/nkh/rdw/internal/bindings"
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
	authed("POST /api/v1/panes/{id}/swap", s.handlePaneSwap)

	// Export (Markdown bundle).
	authed("POST /api/v1/export/pane",   s.handleExportPane)
	authed("POST /api/v1/export/window", s.handleExportWindow)
	authed("POST /api/v1/export/all",    s.handleExportAll)

	// Admin console.
	admin("GET /api/v1/admin/connections", s.handleAdminConnections)

	// Embedded frontend.
	mux.HandleFunc("/", handleFrontend)
}

// routeConfig holds per-route configuration.
type routeConfig struct {
	noAuth         bool
	adminLocalOnly bool
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
	_, exists := s.layouts[name]
	s.layoutMu.RUnlock()

	if !exists {
		apiError(w, http.StatusNotFound, "layout not found: "+name)
		return
	}

	// Applying a saved layout snapshot is a no-op in the current implementation
	// (layouts created interactively are already live). Future: restore from snapshot.
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
