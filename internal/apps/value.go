package apps

import (
	"fmt"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// valueApp shows a single numeric sensor, hidden while it is unavailable.
type valueApp struct {
	cfg   config.App
	topic string
	colorPreview
}

func (a *valueApp) Name() string       { return a.cfg.Name }
func (a *valueApp) Entities() []string { return []string{a.cfg.Entity} }
func (a *valueApp) Ticking() bool      { return false }

func (a *valueApp) Render(st States, _ time.Time) []Message {
	v, ok := numeric(st, a.cfg.Entity)
	if !ok {
		return []Message{remove(a.topic)}
	}
	v *= a.cfg.Scale
	text := fmt.Sprintf(or(a.cfg.Format, "%g"), v)
	color := pickColor(v, a.col("color", a.cfg.Color), a.thresholds(a.cfg.Colors))
	return []Message{show(a.topic, awtrix.AppPayload(a.cfg.Icon, text, color, a.cfg.Extra))}
}
