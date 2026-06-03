// Package server implements the rdw HTTP/WebSocket server.
package server

import (
	"sync"

	"github.com/gorilla/websocket"
)

// wsProto is the WebSocket sub-protocol identifier all clients must present.
const wsProto = "rdw-v1"

// Message is a text frame sent to browser clients.
type Message struct {
	// TargetID identifies the pane this line belongs to.
	TargetID string `json:"target_id"`
	// Line is the rendered text content.
	Line string `json:"line"`
}

// SpecialMessage is a control frame sent to all clients.
type SpecialMessage struct {
	// Type is "reconnect", "window_update", or "pane_update".
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// conn wraps a single WebSocket connection and its authenticated token ID.
type conn struct {
	ws      *websocket.Conn
	tokenID string      // empty means owner connection (Unix socket auth)
	send    chan []byte
	once    sync.Once
}

// close terminates the connection's send goroutine exactly once.
func (c *conn) close() {
	c.once.Do(func() { close(c.send) })
}

// Hub manages all active WebSocket connections and broadcasts messages.
type Hub struct {
	mu      sync.RWMutex
	conns   map[*conn]struct{}
	// revokedTokens tracks tokens that have been revoked so in-flight
	// connections can be dropped on the next write attempt.
	revokedTokens map[string]struct{}
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{
		conns:         make(map[*conn]struct{}),
		revokedTokens: make(map[string]struct{}),
	}
}

// register adds a connection to the hub.
func (h *Hub) register(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[c] = struct{}{}
}

// deregister removes a connection from the hub.
func (h *Hub) deregister(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, c)
}

// Broadcast sends a JSON-encoded message to every connected client.
// Connections whose token has been revoked are dropped before sending.
func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	conns := make([]*conn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		if h.isRevoked(c.tokenID) {
			h.deregister(c)
			c.close()
			continue
		}
		select {
		case c.send <- data:
		default:
			// Slow client: drop connection rather than block broadcast.
			h.deregister(c)
			c.close()
		}
	}
}

// BroadcastSpecial sends a control frame to all clients.
func (h *Hub) BroadcastSpecial(data []byte) {
	h.Broadcast(data)
}

// RevokeToken marks a token as revoked and closes all connections using it.
func (h *Hub) RevokeToken(tokenID string) {
	h.mu.Lock()
	h.revokedTokens[tokenID] = struct{}{}
	var victims []*conn
	for c := range h.conns {
		if c.tokenID == tokenID {
			victims = append(victims, c)
		}
	}
	for _, c := range victims {
		delete(h.conns, c)
	}
	h.mu.Unlock()

	for _, c := range victims {
		c.close()
	}
}

// ConnCount returns the number of active connections.
func (h *Hub) ConnCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

func (h *Hub) isRevoked(tokenID string) bool {
	if tokenID == "" {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.revokedTokens[tokenID]
	return ok
}
