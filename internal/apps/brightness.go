package apps

import (
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// brightnessApp dims the clock while the quiet entity is on.
type brightnessApp struct {
	cfg   config.Brightness
	topic string
}

func (a *brightnessApp) Name() string       { return "brightness" }
func (a *brightnessApp) Entities() []string { return []string{a.cfg.QuietEntity} }
func (a *brightnessApp) Ticking() bool      { return false }

func (a *brightnessApp) Render(st States, _ time.Time) []Message {
	b := a.cfg.Normal
	if isOn(st, a.cfg.QuietEntity) {
		b = a.cfg.Quiet
	}
	return []Message{show(a.topic, awtrix.BrightnessPayload(b))}
}
