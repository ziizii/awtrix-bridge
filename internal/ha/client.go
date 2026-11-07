// Package ha is a minimal Home Assistant WebSocket API client that streams
// entity states.
package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// State is the subset of an HA state object we care about.
type State struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

// Event is delivered to the consumer. Snapshot is true for the full state
// list sent after every (re)connect.
type Event struct {
	Snapshot bool
	States   []State
}

type Client struct {
	wsURL string
	token string
	log   *slog.Logger
}

func New(baseURL, token string, log *slog.Logger) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if !strings.HasSuffix(u.Path, "/api/websocket") {
		u.Path += "/api/websocket"
	}
	return &Client{wsURL: u.String(), token: token, log: log}, nil
}

// Run connects and streams events until ctx is cancelled, reconnecting with backoff.
func (c *Client) Run(ctx context.Context, out chan<- Event) {
	backoff := time.Second
	for {
		start := time.Now()
		err := c.session(ctx, out)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		c.log.Warn("home assistant connection lost", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

type msg struct {
	ID      int             `json:"id,omitempty"`
	Type    string          `json:"type"`
	Success *bool           `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Event   *struct {
		Data struct {
			EntityID string `json:"entity_id"`
			NewState *State `json:"new_state"`
		} `json:"data"`
	} `json:"event,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

func (c *Client) session(ctx context.Context, out chan<- Event) error {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.DefaultDialer.DialContext(dialCtx, c.wsURL, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(64 << 20) // get_states can be large

	// Close the socket when ctx is cancelled so ReadJSON unblocks.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	read := func() (msg, error) {
		var m msg
		err := conn.ReadJSON(&m)
		return m, err
	}

	// Auth handshake.
	if m, err := read(); err != nil || m.Type != "auth_required" {
		return fmt.Errorf("expected auth_required: %v %s", err, m.Type)
	}
	if err := conn.WriteJSON(map[string]string{"type": "auth", "access_token": c.token}); err != nil {
		return err
	}
	if m, err := read(); err != nil {
		return err
	} else if m.Type != "auth_ok" {
		return fmt.Errorf("auth failed: %s %s", m.Type, m.Message)
	}

	// Subscribe first so no change is lost between snapshot and subscription.
	const subID, statesID = 1, 2
	if err := conn.WriteJSON(map[string]any{"id": subID, "type": "subscribe_events", "event_type": "state_changed"}); err != nil {
		return err
	}
	if err := conn.WriteJSON(map[string]any{"id": statesID, "type": "get_states"}); err != nil {
		return err
	}
	c.log.Info("connected to home assistant", "url", c.wsURL)

	// Keepalive pings; HA answers with type "pong".
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		id := 100
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				id++
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(map[string]any{"id": id, "type": "ping"}); err != nil {
					conn.Close()
					return
				}
			}
		}
	}()

	for {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		m, err := read()
		if err != nil {
			return err
		}
		switch m.Type {
		case "result":
			if m.Success != nil && !*m.Success {
				return fmt.Errorf("request %d failed: %+v", m.ID, m.Error)
			}
			if m.ID == statesID {
				var states []State
				if err := json.Unmarshal(m.Result, &states); err != nil {
					return fmt.Errorf("decode states: %w", err)
				}
				if !send(ctx, out, Event{Snapshot: true, States: states}) {
					return ctx.Err()
				}
			}
		case "event":
			if m.Event == nil || m.Event.Data.NewState == nil {
				continue // entity removed
			}
			if !send(ctx, out, Event{States: []State{*m.Event.Data.NewState}}) {
				return ctx.Err()
			}
		}
	}
}

func send(ctx context.Context, out chan<- Event, e Event) bool {
	select {
	case out <- e:
		return true
	case <-ctx.Done():
		return false
	}
}
