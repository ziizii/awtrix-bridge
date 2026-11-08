package apps

import (
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

// tod is a time-of-day sensor: its state, and the window it reports.
type tod struct {
	state         string
	after, before time.Time
}

func (s tod) Get(string) (ha.State, bool) {
	return ha.State{State: s.state, Attributes: map[string]any{
		"after": s.after.Format(time.RFC3339), "before": s.before.Format(time.RFC3339),
	}}, true
}

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
	// Right on the hour, not seconds later.
	if n := len(a.Render(loud, at(10, 0, 0))); n != 1 {
		t.Errorf("10:00:00: want the chime, got %d msgs", n)
	}
	if n := count(loud, at(10, 0, 1), at(10, 30, 0)); n != 0 {
		t.Errorf("chimed again after 10:00")
	}
	if n := count(quiet, at(10, 59, 50), at(11, 0, 30)); n != 0 {
		t.Errorf("quiet time: want silence, got %d", n)
	}
	// A tick that arrives well after the hour (bridge was down) stays silent.
	if n := len(a.Render(loud, at(13, 5, 0))); n != 0 {
		t.Errorf("late tick chimed")
	}
	if n := count(loud, at(13, 59, 59), at(14, 0, 10)); n != 1 {
		t.Errorf("14:00: want 1 chime, got %d", n)
	}
}

// A time-of-day sensor flips exactly on the hour; at hh:00:00 it may still
// report the old state, so its window decides.
func TestChimeQuietWindow(t *testing.T) {
	at := func(d, h int) time.Time { return time.Date(2026, 11, d, h, 0, 0, 0, time.Local) }
	a := &chimeApp{cfg: config.Chime{QuietEntity: "quiet"}, tune: "hour", topic: "snd"}
	a.hour = at(8, 20)
	// 21:00: still "off", the next window starts now: silent.
	if n := len(a.Render(tod{"off", at(8, 21), at(9, 6)}, at(8, 21))); n != 0 {
		t.Errorf("21:00 chimed into quiet time")
	}
	// 06:00: still "on", the window ends now: chime.
	a.hour = at(9, 5)
	if n := len(a.Render(tod{"on", at(8, 21), at(9, 6)}, at(9, 6))); n != 1 {
		t.Errorf("06:00 stayed silent")
	}
	// Midday: off, next window tonight: chime.
	a.hour = at(9, 11)
	if n := len(a.Render(tod{"off", at(9, 21), at(10, 6)}, at(9, 12))); n != 1 {
		t.Errorf("12:00 stayed silent")
	}
}
