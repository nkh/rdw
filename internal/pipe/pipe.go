// Package pipe implements the client-side stdin relay for rdw pipe.
// It reads lines from a reader and forwards each to the server via the Unix
// domain socket (preferred) or the REST API.
package pipe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nkh/rdw/internal/server"
	"github.com/nkh/rdw/internal/session"
)

// Options configures a Relay.
type Options struct {
	// TargetID is the pane to route data into.
	TargetID session.TargetID

	// Port is the server port (used when SocketPath is empty).
	Port int

	// SocketPath is the Unix domain socket path. When set, data is sent
	// via the Unix socket rather than HTTP. This is used for local same-user
	// connections and requires no token.
	SocketPath string

	// Token is the Bearer token for REST API auth. Ignored when SocketPath is set.
	Token string

	// ReconnectQueueLen is the number of lines buffered while disconnected.
	// Default: 1000.
	ReconnectQueueLen int
}

// Relay reads lines from r and forwards each line to the server.
// It returns when r is exhausted or ctx is cancelled.
func Relay(ctx context.Context, r io.Reader, opts Options) error {
	if opts.ReconnectQueueLen == 0 {
		opts.ReconnectQueueLen = 1000
	}

	var sendFn func(line string) error

	if opts.SocketPath != "" {
		sendFn = func(line string) error {
			return sendViaUnix(opts.SocketPath, opts.TargetID, line)
		}
	} else {
		sendFn = func(line string) error {
			return sendViaHTTP(opts.Port, opts.TargetID, opts.Token, line)
		}
	}

	return relay(ctx, r, sendFn, opts.ReconnectQueueLen)
}

// relay is the core loop: read lines, buffer on error, flush and continue.
func relay(ctx context.Context, r io.Reader, send func(string) error, queueLen int) error {
	scanner := bufio.NewScanner(r)
	queue := make([]string, 0, 64)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// Flush buffered lines first.
		var i int
		for i < len(queue) {
			if err := send(queue[i]); err != nil {
				// Still disconnected — keep buffering.
				break
			}
			i++
		}
		queue = queue[i:]

		// Send the current line.
		if err := send(line); err != nil {
			// Buffer it if there is room.
			if len(queue) < queueLen {
				queue = append(queue, line)
			}
			// Drop oldest if overflow — silent, matches spec.
		}
	}

	return scanner.Err()
}

// sendViaUnix sends a single line through the server's Unix domain socket.
func sendViaUnix(socketPath string, id session.TargetID, line string) error {
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		return fmt.Errorf("unix socket %q: %w", socketPath, err)
	}
	defer conn.Close()

	params, _ := json.Marshal(map[string]string{
		"target_id": id.String(),
		"line":      line,
	})

	cmd := server.UnixCommand{Action: "stream", Params: params}
	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		return fmt.Errorf("sending command: %w", err)
	}

	var resp server.UnixResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("server: %s", resp.Error)
	}

	return nil
}

// sendViaHTTP sends a single line to POST /api/v1/stream/:id.
func sendViaHTTP(port int, id session.TargetID, token, line string) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/stream/%s", port, id.String())

	body, _ := json.Marshal(map[string]string{"line": line})
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	return nil
}
