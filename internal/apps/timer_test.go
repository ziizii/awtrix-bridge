package apps

import (
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

func TestTimer(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	a := &timerApp{topics: topics, cfg: config.Timer{
		Icon: "1", Color: "#FFFFFF", DoneColor: "#FF0000", Melody: "ding", HoldAfter: config.Duration(time.Minute),
	}}
	byTopic := func(msgs []Message) map[string]Message {
		out := map[string]Message{}
		for _, m := range msgs {
			out[m.Topic] = m
		}
		return out
	}
	const app, sw = "awtrix/cmd/apps/pushed/timer", "awtrix/cmd/apps/switch"

	// No timer: the app is removed, nothing else.
	if msgs := a.Render(nil, now); len(msgs) != 1 || msgs[0].Topic != app || msgs[0].Payload != nil {
		t.Fatalf("idle: %v", msgs)
	}

	// Started: shows the time left with progress and switches to itself once.
	a.SetTimer(awtrix.Timer{Start: now, End: now.Add(90 * time.Second), Back: "temp_out"})
	m := byTopic(a.Render(nil, now))
	if p := decode(t, m[app].Payload); p["text"] != "1:30" || p["progress"] != 0.0 {
		t.Errorf("start payload %v", p)
	}
	if string(m[sw].Payload) != `{"fast":true,"name":"timer"}` {
		t.Errorf("start switch %q", m[sw].Payload)
	}
	m = byTopic(a.Render(nil, now.Add(45*time.Second)))
	if p := decode(t, m[app].Payload); p["text"] != "0:45" || p["progress"] != 50.0 {
		t.Errorf("half payload %v", p)
	}
	if _, ok := m[sw]; ok {
		t.Errorf("switched twice")
	}

	// End: rings once, counts up in the done color.
	m = byTopic(a.Render(nil, now.Add(90*time.Second)))
	if _, ok := m["snd"]; !ok {
		t.Errorf("no sound at the end")
	}
	if _, ok := byTopic(a.Render(nil, now.Add(91*time.Second)))["snd"]; ok {
		t.Errorf("rang twice")
	}
	for s, want := range map[int]string{92: "+0:02", 93: "+0:03"} { // steady, no blinking
		if p := decode(t, a.Render(nil, now.Add(time.Duration(s)*time.Second))[0].Payload); p["text"] != want || p["textColor"] != "#FF0000" {
			t.Errorf("count-up payload %v", p)
		}
	}

	// After hold_after: switches back, removes the app, clears the state.
	m = byTopic(a.Render(nil, now.Add(151*time.Second)))
	if string(m[sw].Payload) != `{"fast":true,"name":"temp_out"}` || m[app].Payload != nil {
		t.Errorf("end msgs %v", m)
	}
	if c := m["awtrix-bridge/timer"]; !c.Retain || len(c.Payload) != 0 {
		t.Errorf("state not cleared: %+v", c)
	}
	if a.Timer().Running() {
		t.Errorf("still running")
	}
}

func TestTimerCancelAndRestart(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	a := &timerApp{topics: topics, cfg: config.Timer{Melody: "ding", HoldAfter: config.Duration(time.Minute)}}
	a.SetTimer(awtrix.Timer{Start: now, End: now.Add(time.Hour + 5*time.Second)})
	if p := decode(t, a.Render(nil, now)[0].Payload); p["text"] != "1:00:05" {
		t.Errorf("hour payload %v", p)
	}

	// Cancelled: back to Time (no app recorded), once.
	a.SetTimer(awtrix.Timer{})
	msgs := a.Render(nil, now.Add(time.Second))
	if len(msgs) != 2 || string(msgs[0].Payload) != `{"fast":true,"name":"Time"}` {
		t.Errorf("cancel: %v", msgs)
	}
	if msgs := a.Render(nil, now.Add(2*time.Second)); len(msgs) != 1 {
		t.Errorf("switched back twice: %v", msgs)
	}

	// A timer found after a restart neither switches nor rings late.
	b := &timerApp{topics: topics, cfg: config.Timer{Melody: "ding", HoldAfter: config.Duration(5 * time.Minute)}}
	b.SetTimer(awtrix.Timer{Start: now.Add(-time.Hour), End: now.Add(-2 * time.Minute)})
	for _, m := range b.Render(nil, now) {
		if m.Topic == "snd" || m.Topic == "awtrix/cmd/apps/switch" {
			t.Errorf("restart sent %s", m.Topic)
		}
	}
}

// awtrixng rejects payloads with keys it does not know.
func TestTimerPayloadKeys(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	a := &timerApp{topics: topics, cfg: config.Timer{Icon: "1", Color: "#FFFFFF"}}
	a.SetTimer(awtrix.Timer{Start: now, End: now.Add(time.Minute)})
	known := map[string]bool{"text": true, "icon": true, "textColor": true, "progress": true, "progressColor": true, "progressTrackColor": true}
	p := decode(t, a.Render(nil, now)[0].Payload)
	for k := range p {
		if !known[k] {
			t.Errorf("payload key %q", k)
		}
	}
	// The firmware's track is white by default: a white bar on it is invisible.
	if p["progressTrackColor"] == nil || p["progressTrackColor"] == p["progressColor"] {
		t.Errorf("bar indistinguishable from its track: %v", p)
	}
}
