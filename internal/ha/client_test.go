package ha

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeHA speaks enough of the HA WebSocket protocol for the client.
func fakeHA(t *testing.T) *httptest.Server {
	up := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/websocket" {
			http.NotFound(w, r)
			return
		}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		c.WriteJSON(map[string]string{"type": "auth_required"})
		var auth map[string]string
		c.ReadJSON(&auth)
		if auth["access_token"] != "secret" {
			c.WriteJSON(map[string]string{"type": "auth_invalid", "message": "bad token"})
			return
		}
		c.WriteJSON(map[string]string{"type": "auth_ok"})
		for i := 0; i < 2; i++ {
			var req map[string]any
			c.ReadJSON(&req)
			id := req["id"]
			switch req["type"] {
			case "subscribe_events":
				c.WriteJSON(map[string]any{"id": id, "type": "result", "success": true})
			case "get_states":
				c.WriteJSON(map[string]any{"id": id, "type": "result", "success": true,
					"result": []map[string]any{{"entity_id": "sensor.a", "state": "1", "attributes": map[string]any{}}}})
			}
		}
		c.WriteJSON(map[string]any{"id": 1, "type": "event", "event": map[string]any{"data": map[string]any{
			"entity_id": "sensor.a", "new_state": map[string]any{"entity_id": "sensor.a", "state": "2"}}}})
		time.Sleep(time.Second)
	}))
}

func TestClientStreams(t *testing.T) {
	srv := fakeHA(t)
	defer srv.Close()
	c, err := New(srv.URL, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := make(chan Event, 4)
	go c.Run(ctx, out)

	snap := <-out
	if !snap.Snapshot || len(snap.States) != 1 || snap.States[0].State != "1" {
		t.Fatalf("snapshot: %+v", snap)
	}
	ev := <-out
	if ev.Snapshot || ev.States[0].State != "2" {
		t.Fatalf("event: %+v", ev)
	}
}

func TestURL(t *testing.T) {
	c, _ := New("https://ha.example.com:8123/", "x", nil)
	if c.wsURL != "wss://ha.example.com:8123/api/websocket" {
		t.Errorf("got %s", c.wsURL)
	}
}
