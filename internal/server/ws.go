package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024 // 512 KB
	sendBufSize    = 256
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Enforce sub-protocol rdw-v1.
	Subprotocols: []string{wsProto},
	CheckOrigin: func(r *http.Request) bool {
		// Origin check is intentionally permissive for local use.
		// Production deployments should restrict this.
		return true
	},
}

// serveWS upgrades an HTTP connection to WebSocket and registers it with
// the hub. tokenID is the authenticated token (empty for owner connections).
func serveWS(hub *Hub, w http.ResponseWriter, r *http.Request, tokenID string) {
	// Reject clients that did not negotiate rdw-v1.
	negotiated := websocket.Subprotocols(r)
	found := false
	for _, p := range negotiated {
		if p == wsProto {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "WebSocket sub-protocol rdw-v1 required", http.StatusBadRequest)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	c := &conn{
		ws:      ws,
		tokenID: tokenID,
		send:    make(chan []byte, sendBufSize),
	}

	hub.register(c)

	go writePump(c)
	go readPump(hub, c)

	// Send RECONNECT marker so the client flushes its queue.
	reconnectMsg, _ := json.Marshal(SpecialMessage{Type: "reconnect"})
	select {
	case c.send <- reconnectMsg:
	default:
	}
}

// readPump drains incoming frames (only pings/pongs expected from clients).
// When the connection closes, the conn is deregistered.
func readPump(hub *Hub, c *conn) {
	defer func() {
		hub.deregister(c)
		c.ws.Close()
	}()

	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, _, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
	}
}

// writePump drains the send channel and forwards frames to the client.
func writePump(c *conn) {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// Channel closed — send close frame and exit.
				_ = c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
