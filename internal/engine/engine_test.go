package engine

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

type recorder struct{ msgs []string }

func (r *recorder) Publish(topic string, p []byte, _ bool) error {
	r.msgs = append(r.msgs, topic+" "+string(p))
	return nil
}

func TestDedupeAndRouting(t *testing.T) {
	cfg := &config.Config{
		Awtrix: config.Awtrix{AppTopic: "a/{name}"},
		Apps: []config.App{
			{Name: "solar", Type: "value", Entity: "sensor.pv", Format: "%.0fW", Scale: 1},
			{Name: "temp", Type: "value", Entity: "sensor.t", Format: "%.1f", Scale: 1},
		},
	}
	list, err := apps.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	e := New(list, r, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now()

	e.handle(ha.Event{Snapshot: true, States: []ha.State{{EntityID: "sensor.pv", State: "100"}, {EntityID: "sensor.t", State: "20"}}}, now)
	if len(r.msgs) != 2 {
		t.Fatalf("snapshot: %v", r.msgs)
	}
	// Same value -> no publish; unrelated entity -> no publish.
	e.handle(ha.Event{States: []ha.State{{EntityID: "sensor.pv", State: "100"}, {EntityID: "sensor.other", State: "1"}}}, now)
	if len(r.msgs) != 2 {
		t.Fatalf("dedupe failed: %v", r.msgs)
	}
	e.handle(ha.Event{States: []ha.State{{EntityID: "sensor.pv", State: "250"}}}, now)
	if len(r.msgs) != 3 || r.msgs[2] != `a/solar {"text":"250W"}` {
		t.Fatalf("change not published: %v", r.msgs)
	}
	e.republish()
	if len(r.msgs) != 5 {
		t.Fatalf("resync should republish all: %v", r.msgs)
	}
}
