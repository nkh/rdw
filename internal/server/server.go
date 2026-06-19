package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sync"

	"github.com/nkh/rdw/internal/auth"
	"github.com/nkh/rdw/internal/config"
	"github.com/nkh/rdw/internal/control"
	"github.com/nkh/rdw/internal/discovery"
	"github.com/nkh/rdw/internal/highlight"
	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/terminal"
	"github.com/nkh/rdw/internal/router"
	"github.com/nkh/rdw/internal/session"
)

const shutdownTimeout = 10 * time.Second

// Options configures a Server beyond what is in Config.
type Options struct {
	SessionID   string
	PersistPath string // path to SQLite DB for KV persistence; empty = memory-only
	Restore     bool   // load persisted KV on startup
}

// Server is the running rdw server instance.
type Server struct {
	cfg        config.Config
	opts       Options
	hub        *Hub
	tokenStore *auth.Store
	rl         *RateLimiter
	kv         *kvstore.Store
	kvDB       *kvstore.DB // non-nil when --kv-persist is set
	highlights *highlight.Store
	terminals  *terminal.Manager
	cycleCancel context.CancelFunc // non-nil while a focus cycle is running; guarded by cycleMu
	cycleMu     sync.Mutex
	runCtx      context.Context // cancelled when server shuts down
	manager    *session.Manager
	router     *router.Router
	httpSrv    *http.Server
	unixLn     net.Listener
	socketPath string
	port       int

	layoutMu sync.RWMutex
	layouts  map[string][]byte // name -> JSON snapshot
}

// New creates a Server from the given config. Call Run to start it.
func New(cfg config.Config, opts Options) *Server {
	port := cfg.Server.Port

	sessionID := opts.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("%d", port)
	}

	kv := kvstore.New()
	mgr := session.NewManager(kv)

	s := &Server{
		cfg:        cfg,
		opts:       Options{SessionID: sessionID},
		hub:        NewHub(),
		tokenStore: auth.NewStore(),
		rl:         NewRateLimiter(),
		kv:         kv,
		highlights: highlight.New(),
		terminals:  terminal.New(),
		manager:    mgr,
		port:       port,
		layouts:    make(map[string][]byte),
		runCtx:     context.Background(),
	}

	// Create router that broadcasts rendered lines to WebSocket clients.
	s.router = router.New(kv, func(id session.TargetID, line string) {
		data, _ := json.Marshal(Message{TargetID: id.String(), Line: line})
		s.hub.Broadcast(data)
	}, router.Options{
		AllowUnassigned:      false,
		FilterChainMax:       cfg.Server.FilterChainMax,
		DefaultScrollbackCap: cfg.Server.ScrollbackCap,
	})

	// Wire control sequence handler for bm:, hl:, sc: sequences.
	s.router.SetControlHandler(func(id session.TargetID, seq control.Sequence) {
		switch seq.Kind {
		case control.KindBookmark:
			if bs := s.router.Bookmarks(id); bs != nil && seq.Payload != "" {
				// Line index is the current scrollback length.
				pl, ok := s.router.Get(id)
				lineIdx := 0
				if ok {
					lineIdx = pl.Scrollback().Len()
				}
				_ = bs.Add(seq.Payload, lineIdx)
			}

		case control.KindHighlight:
			// hl:<profile-name> — send a highlight_set message to the browser.
			if seq.Payload != "" {
				if p, ok := s.highlights.Get(seq.Payload); ok {
					type hlMsg struct {
						Type     string      `json:"type"`
						TargetID string      `json:"target_id"`
						Profile  interface{} `json:"profile"`
					}
					data, _ := json.Marshal(hlMsg{
						Type:     "highlight_set",
						TargetID: id.String(),
						Profile:  p,
					})
					s.hub.Broadcast(data)
				}
			}

		case control.KindScrollback:
			// sc:clear|top|bottom — send a scrollback_ctl message to the browser.
			if seq.Payload != "" {
				// On clear, also wipe the server-side scrollback buffer.
				if seq.Payload == "clear" {
					if pl, ok := s.router.Get(id); ok {
						pl.Scrollback().Clear()
					}
				}

				type scMsg struct {
					Type     string `json:"type"`
					TargetID string `json:"target_id"`
					Action   string `json:"action"`
				}
				data, _ := json.Marshal(scMsg{
					Type:     "scrollback_ctl",
					TargetID: id.String(),
					Action:   seq.Payload,
				})
				s.hub.Broadcast(data)
			}
		}
	})

	return s
}

// Run starts the HTTP server and blocks until context cancelled or OS signal.
func (s *Server) Run(ctx context.Context) error {
	s.runCtx = ctx

	if s.opts.PersistPath != "" {
		db, err := kvstore.OpenDB(s.opts.PersistPath)
		if err != nil {
			return fmt.Errorf("kv-persist: %w", err)
		}

		s.kvDB = db

		if s.opts.Restore {
			if err := db.Load(s.kv); err != nil {
				fmt.Fprintf(os.Stderr, "rdw: warning: restore failed: %v\n", err)
			}
		}
	}

	ln, sockPath, err := startUnixListener(s.opts.SessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rdw: warning: unix socket unavailable: %v\n", err)
	} else {
		s.unixLn = ln
		s.socketPath = sockPath
		go acceptUnix(ln, s.handleUnixCommand)
	}

	mux := http.NewServeMux()
	routes(mux, s, routeConfig{
		noAuth:         s.cfg.Auth.NoAuth,
		adminLocalOnly: s.cfg.Auth.AdminLocalOnly,
	})

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	if s.cfg.Server.NetworkExpose {
		addr = fmt.Sprintf(":%d", s.port)
	}

	s.httpSrv = &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 120 * time.Second,
	}

	_ = discovery.Register(discovery.ServerInfo{
		Port:       s.port,
		PID:        os.Getpid(),
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		SocketPath: s.socketPath,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "rdw: listening on %s\n", addr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		s.cleanup()
		return fmt.Errorf("server error: %w", err)
	case <-sigCh:
		fmt.Fprintln(os.Stderr, "rdw: shutting down")
	case <-ctx.Done():
	}

	return s.shutdown()
}

func (s *Server) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	err := s.httpSrv.Shutdown(ctx)
	s.cleanup()
	return err
}

func (s *Server) cleanup() {
	if s.unixLn != nil {
		_ = s.unixLn.Close()
	}
	if s.socketPath != "" {
		_ = os.Remove(s.socketPath)
	}
	_ = discovery.Deregister(s.port)

	if s.kvDB != nil {
		_ = s.kvDB.Close()
	}
}

// Accessors used by tests and CLI wiring.
func (s *Server) Port() int               { return s.port }
func (s *Server) Hub() *Hub               { return s.hub }
func (s *Server) TokenStore() *auth.Store { return s.tokenStore }
func (s *Server) Manager() *session.Manager { return s.manager }
func (s *Server) Router() *router.Router  { return s.router }
func (s *Server) KV() *kvstore.Store      { return s.kv }

// HTTPHandler returns the http.Handler for testing with httptest.NewServer.
func (s *Server) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	routes(mux, s, routeConfig{
		noAuth:         s.cfg.Auth.NoAuth,
		adminLocalOnly: s.cfg.Auth.AdminLocalOnly,
	})
	return mux
}

// StartUnixSocket starts only the Unix socket (for tests).
func (s *Server) StartUnixSocket() (string, error) {
	ln, path, err := startUnixListener(s.opts.SessionID)
	if err != nil {
		return "", err
	}
	s.unixLn = ln
	s.socketPath = path
	go acceptUnix(ln, s.handleUnixCommand)
	return path, nil
}

// StopUnixSocket closes the Unix socket listener.
func (s *Server) StopUnixSocket() {
	if s.unixLn != nil {
		_ = s.unixLn.Close()
	}
	if s.socketPath != "" {
		_ = os.Remove(s.socketPath)
	}
}

// handleUnixCommand handles commands arriving on the Unix domain socket.
func (s *Server) handleUnixCommand(cmd UnixCommand) UnixResponse {
	switch cmd.Action {
	case "ping":
		return UnixResponse{OK: true, Data: map[string]interface{}{
			"port":       s.port,
			"pid":        os.Getpid(),
			"started_at": time.Now().UTC().Format(time.RFC3339),
		}}

	case "stop":
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if s.httpSrv != nil {
				_ = s.httpSrv.Shutdown(ctx)
			}
			s.cleanup()
		}()
		return UnixResponse{OK: true}

	case "token.create":
		var params struct {
			Expiry  int64    `json:"expiry_seconds"`
			Panes   []string `json:"panes"`
			Windows []string `json:"windows"`
		}
		if err := json.Unmarshal(cmd.Params, &params); err != nil {
			return UnixResponse{OK: false, Error: err.Error()}
		}
		expiry := time.Duration(params.Expiry) * time.Second
		if params.Expiry == 0 {
			expiry = config.DefaultTokenExpiry
		}
		plain, tok, err := s.tokenStore.Create(auth.CreateOptions{
			Expiry:  expiry,
			Panes:   params.Panes,
			Windows: params.Windows,
		})
		if err != nil {
			return UnixResponse{OK: false, Error: err.Error()}
		}
		return UnixResponse{OK: true, Data: map[string]interface{}{
			"token":      plain,
			"id":         tok.ID,
			"expires_at": tok.ExpiresAt,
		}}

	case "token.revoke":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(cmd.Params, &params); err != nil {
			return UnixResponse{OK: false, Error: err.Error()}
		}
		s.hub.RevokeToken(params.ID)
		if !s.tokenStore.Revoke(params.ID) {
			return UnixResponse{OK: false, Error: "token not found"}
		}
		return UnixResponse{OK: true}

	case "stream":
		var params struct {
			TargetID string `json:"target_id"`
			Line     string `json:"line"`
		}
		if err := json.Unmarshal(cmd.Params, &params); err != nil {
			return UnixResponse{OK: false, Error: err.Error()}
		}
		id, err := session.ParseTargetID(params.TargetID)
		if err != nil {
			return UnixResponse{OK: false, Error: err.Error()}
		}
		if err := s.router.Route(id, params.Line); err != nil {
			return UnixResponse{OK: false, Error: err.Error()}
		}
		return UnixResponse{OK: true}

	default:
		return UnixResponse{OK: false, Error: fmt.Sprintf("unknown action: %q", cmd.Action)}
	}
}
