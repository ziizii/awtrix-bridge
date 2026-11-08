package apps

import (
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

func TestChime(t *testing.T) {
	a := &chimeApp{cfg: config.Chime{QuietEntity: "quiet"}, tune: "hour", topic: "snd"}
	loud, quiet := fake{"quiet": "off"}, fake{"quiet": "on"}
	at := func(h, m, s int) time.Time { return time.Date(2026, 11, 8, h, m, s, 0, time.Local) }
	count := func(st States, from, to time.Time) (n int) {
		for now := from; !now.After(to); now = now.Add(time.Second) {
			n += len(a.Render(st, now))
		}
		return
	}
	if n := count(loud, at(9, 41, 0), at(9, 59, 59)); n != 0 {
		t.Errorf("chimed %d times after start, before the hour", n)
	}
	if n := count(loud, at(10, 0, 0), at(10, 30, 0)); n != 1 {
		t.Errorf("10:00: want 1 chime, got %d", n)
	}
	if n := count(quiet, at(10, 59, 50), at(11, 0, 30)); n != 0 {
		t.Errorf("quiet time: want silence, got %d", n)
	}
	// Quiet time switching on exactly at the hour: the chime waits a few
	// seconds, so it sees the new state.
	a.Render(loud, at(11, 59, 59))
	a.Render(loud, at(12, 0, 0))
	if n := count(quiet, at(12, 0, 1), at(12, 0, 30)); n != 0 {
		t.Errorf("quiet from 12:00:00: want silence, got %d", n)
	}
	// A tick that arrives well after the hour (bridge was down) stays silent.
	if n := len(a.Render(loud, at(13, 5, 0))); n != 0 {
		t.Errorf("late tick chimed")
	}
	if n := count(loud, at(13, 59, 59), at(14, 0, 10)); n != 1 {
		t.Errorf("14:00: want 1 chime, got %d", n)
	}
}
