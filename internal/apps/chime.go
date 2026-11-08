package apps

import (
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// chimeApp plays a melody right as a new hour starts (the engine ticks on
// whole seconds), unless quiet time is on at that hour.
type chimeApp struct {
	cfg   config.Chime
	tune  string
	topic string
	hour  time.Time // hour already handled
}

// A tick later than chimeLate after the hour (the bridge was down)
// stays silent.
const chimeLate = time.Minute

func (a *chimeApp) Name() string { return "chime" }
func (a *chimeApp) Entities() []string {
	if a.cfg.QuietEntity == "" {
		return nil
	}
	return []string{a.cfg.QuietEntity}
}
func (a *chimeApp) Ticking() bool { return true }

func (a *chimeApp) Render(st States, now time.Time) []Message {
	hour := now.Truncate(time.Hour)
	if a.hour.IsZero() {
		a.hour = hour // never chime just because the bridge started
		return nil
	}
	if !hour.After(a.hour) {
		return nil
	}
	a.hour = hour
	if now.Sub(hour) > chimeLate || quietAt(st, a.cfg.QuietEntity, hour) {
		return nil
	}
	return []Message{event(a.topic, awtrix.SoundPayload(a.tune))}
}

// quietAt reports whether the quiet entity is on at t. A time-of-day
// sensor (HA "tod") flips exactly on the hour, so at hh:00:00 its new
// state may not have arrived yet; its after/before attributes give the
// window (the current one while on, the next while off), which decides
// instead. Other entities go by their state.
func quietAt(st States, id string, t time.Time) bool {
	if id == "" {
		return false
	}
	s, ok := st.Get(id)
	if !ok {
		return false
	}
	after, ok1 := attrTime(s.Attributes, "after")
	before, ok2 := attrTime(s.Attributes, "before")
	if ok1 && ok2 {
		return !t.Before(after) && t.Before(before)
	}
	return s.State == "on"
}

func attrTime(attrs map[string]any, key string) (time.Time, bool) {
	v, _ := attrs[key].(string)
	t, err := time.Parse(time.RFC3339, v)
	return t, err == nil
}
