package apps

import (
	"fmt"
	"math"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// surplusApp shows production minus consumption, e.g. "+1.2kW" / "-850W".
type surplusApp struct {
	cfg   config.App
	topic string
	colorPreview
}

func (a *surplusApp) Name() string       { return a.cfg.Name }
func (a *surplusApp) Entities() []string { return []string{a.cfg.Production, a.cfg.Consumption} }
func (a *surplusApp) Ticking() bool      { return false }

func (a *surplusApp) Render(st States, _ time.Time) []Message {
	pv, ok1 := numeric(st, a.cfg.Production)
	load, ok2 := numeric(st, a.cfg.Consumption)
	if !ok1 || !ok2 {
		return []Message{remove(a.topic)}
	}
	surplus := pv - load
	icon := a.cfg.Icon
	if pv == 0 && a.cfg.NightIcon != "" {
		icon = a.cfg.NightIcon
	}
	color := a.col("positive_color", or(a.cfg.PositiveColor, "#00FF00"))
	if surplus < 0 || a.lastKey == "negative_color" { // show the color being picked
		color = a.col("negative_color", or(a.cfg.NegativeColor, "#FF4400"))
	}
	return []Message{show(a.topic, awtrix.AppPayload(icon, FormatPower(surplus, true), color, a.cfg.Extra))}
}

// FormatPower renders watts as "850W" or "1.2kW", optionally with a sign.
func FormatPower(w float64, signed bool) string {
	sign := ""
	if signed && w > 0 {
		sign = "+"
	} else if w < 0 {
		sign = "-"
	}
	abs := math.Abs(w)
	if abs >= 1000 {
		return fmt.Sprintf("%s%.1fkW", sign, abs/1000)
	}
	return fmt.Sprintf("%s%dW", sign, int(abs))
}
