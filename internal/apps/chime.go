package apps

import (
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// chimeApp plays a melody when a new hour starts, unless quiet time is on.
type chimeApp struct {
	cfg   config.Chime
	tune  string
	topic string
	hour  time.Time // hour already handled
}

// The chime waits chimeDelay after the full hour so a quiet-time entity
// that flips exactly on the hour (e.g. 21:00) is already updated. A tick
// later than chimeLate (the bridge was down) stays silent.
const (
	chimeDelay = 5 * time.Second
	chimeLate  = time.Minute
)

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
	since := now.Sub(hour)
	if !hour.After(a.hour) || since < chimeDelay {
		return nil
	}
	a.hour = hour
	if since > chimeLate || isOn(st, a.cfg.QuietEntity) {
		return nil
	}
	return []Message{event(a.topic, awtrix.SoundPayload(a.tune))}
}
