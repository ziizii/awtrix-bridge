// Package apps turns Home Assistant states into AWTRIX MQTT messages.
package apps

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

// States gives read access to the latest known entity states.
type States interface {
	Get(entityID string) (ha.State, bool)
}

// Message is one MQTT publish. Payload nil means "remove the app" (empty
// payload). Once messages (sounds, app switches) are never deduplicated
// or re-sent on resync.
type Message struct {
	Topic   string
	Payload []byte
	Once    bool
}

type App interface {
	Name() string
	// Entities lists the entity IDs whose changes trigger a re-render.
	Entities() []string
	// Ticking apps are also re-rendered every second.
	Ticking() bool
	Render(st States, now time.Time) []Message
}

type Topics struct {
	App      string // contains {name}
	Settings string
	Sound    string
	Switch   string
}

func (t Topics) app(name string) string { return strings.ReplaceAll(t.App, "{name}", name) }

// Build creates the apps described in the config.
func Build(c *config.Config) ([]App, error) {
	t := Topics{
		App:      c.Awtrix.AppTopic,
		Settings: c.Awtrix.SettingsTopic,
		Sound:    c.Awtrix.SoundTopic,
		Switch:   switchTopic(c.Awtrix.AppTopic),
	}
	var out []App
	if b := c.Brightness; b != nil {
		out = append(out, &brightnessApp{cfg: *b, topic: t.Settings})
	}
	for _, a := range c.Apps {
		switch a.Type {
		case "value":
			out = append(out, &valueApp{cfg: a, topic: t.app(a.Name)})
		case "surplus":
			out = append(out, &surplusApp{cfg: a, topic: t.app(a.Name)})
		case "countdown":
			out = append(out, &countdownApp{cfg: a, topics: t})
		default:
			return nil, fmt.Errorf("unknown app type %q", a.Type)
		}
	}
	return out, nil
}

// switchTopic derives ".../apps/switch" from ".../apps/pushed/{name}".
func switchTopic(appTopic string) string {
	if i := strings.Index(appTopic, "/pushed/"); i >= 0 {
		return appTopic[:i] + "/switch"
	}
	return ""
}

// ---- helpers -----------------------------------------------------------

func numeric(st States, id string) (float64, bool) {
	s, ok := st.Get(id)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(s.State, 64)
	if err != nil || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

func pickColor(v float64, fallback string, th []config.Threshold) string {
	for _, t := range th {
		if t.Below == nil || v < *t.Below {
			return t.Color
		}
	}
	return fallback
}

func payload(icon, text, color string, extra map[string]any) []byte {
	m := map[string]any{"text": text}
	if icon != "" {
		m["icon"] = icon
	}
	if color != "" {
		m["textColor"] = color
	}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}

func remove(topic string) []Message { return []Message{{Topic: topic, Payload: nil}} }

// ---- brightness --------------------------------------------------------

type brightnessApp struct {
	cfg   config.Brightness
	topic string
}

func (a *brightnessApp) Name() string       { return "brightness" }
func (a *brightnessApp) Entities() []string { return []string{a.cfg.QuietEntity} }
func (a *brightnessApp) Ticking() bool      { return false }
func (a *brightnessApp) Render(st States, _ time.Time) []Message {
	b := a.cfg.Normal
	if s, ok := st.Get(a.cfg.QuietEntity); ok && s.State == "on" {
		b = a.cfg.Quiet
	}
	p, _ := json.Marshal(map[string]any{"brightness": b, "autoBrightness": false})
	return []Message{{Topic: a.topic, Payload: p}}
}

// ---- value -------------------------------------------------------------

// valueApp shows a single numeric sensor, hidden while it is unavailable.
type valueApp struct {
	cfg   config.App
	topic string
}

func (a *valueApp) Name() string       { return a.cfg.Name }
func (a *valueApp) Entities() []string { return []string{a.cfg.Entity} }
func (a *valueApp) Ticking() bool      { return false }
func (a *valueApp) Render(st States, _ time.Time) []Message {
	v, ok := numeric(st, a.cfg.Entity)
	if !ok {
		return remove(a.topic)
	}
	v *= a.cfg.Scale
	format := a.cfg.Format
	if format == "" {
		format = "%g"
	}
	text := fmt.Sprintf(format, v)
	return []Message{{Topic: a.topic, Payload: payload(a.cfg.Icon, text, pickColor(v, a.cfg.Color, a.cfg.Colors), a.cfg.Extra)}}
}

// ---- surplus -----------------------------------------------------------

// surplusApp shows production minus consumption, e.g. "+1.2kW" / "-850W".
type surplusApp struct {
	cfg   config.App
	topic string
}

func (a *surplusApp) Name() string       { return a.cfg.Name }
func (a *surplusApp) Entities() []string { return []string{a.cfg.Production, a.cfg.Consumption} }
func (a *surplusApp) Ticking() bool      { return false }
func (a *surplusApp) Render(st States, _ time.Time) []Message {
	pv, ok1 := numeric(st, a.cfg.Production)
	load, ok2 := numeric(st, a.cfg.Consumption)
	if !ok1 || !ok2 {
		return remove(a.topic)
	}
	surplus := pv - load
	icon := a.cfg.Icon
	if pv == 0 && a.cfg.NightIcon != "" {
		icon = a.cfg.NightIcon
	}
	color := or(a.cfg.PositiveColor, "#00FF00")
	if surplus < 0 {
		color = or(a.cfg.NegativeColor, "#FF4400")
	}
	return []Message{{Topic: a.topic, Payload: payload(icon, FormatPower(surplus, true), color, a.cfg.Extra)}}
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

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---- countdown ---------------------------------------------------------

// countdownApp counts down to a timestamp entity (e.g. next bus arrival),
// updating every second while the target is within ShowWithin.
type countdownApp struct {
	cfg     config.App
	topics  Topics
	visible bool
	alerted time.Time // target for which the alert already played
}

func (a *countdownApp) Name() string       { return a.cfg.Name }
func (a *countdownApp) Entities() []string { return []string{a.cfg.Entity} }
func (a *countdownApp) Ticking() bool      { return true }

func (a *countdownApp) Render(st States, now time.Time) []Message {
	topic := a.topics.app(a.cfg.Name)
	s, ok := st.Get(a.cfg.Entity)
	if !ok {
		return a.hide(topic)
	}
	target, err := time.Parse(time.RFC3339, s.State)
	if err != nil {
		return a.hide(topic)
	}
	remaining := target.Sub(now)
	if remaining < -a.cfg.HoldAfter.D() || (a.cfg.ShowWithin > 0 && remaining > a.cfg.ShowWithin.D()) {
		return a.hide(topic)
	}

	var text string
	secs := int(math.Ceil(remaining.Seconds()))
	if secs <= 0 {
		text = "Now!"
	} else {
		text = fmt.Sprintf("%d:%02d", secs/60, secs%60)
	}
	mins := float64(secs) / 60
	msgs := []Message{{Topic: topic, Payload: payload(a.cfg.Icon, text, pickColor(mins, a.cfg.Color, a.cfg.Colors), a.cfg.Extra)}}

	if !a.visible && a.cfg.SwitchTo && a.topics.Switch != "" {
		p, _ := json.Marshal(map[string]any{"name": a.cfg.Name, "fast": true})
		// The switch must follow the first app payload so the app exists.
		msgs = append(msgs, Message{Topic: a.topics.Switch, Payload: p, Once: true})
	}
	a.visible = true

	if a.cfg.AlertAt > 0 && a.cfg.AlertRTTTL != "" && remaining > 0 &&
		remaining <= a.cfg.AlertAt.D() && !a.alerted.Equal(target) {
		a.alerted = target
		p, _ := json.Marshal(map[string]string{"rtttl": a.cfg.AlertRTTTL})
		msgs = append(msgs, Message{Topic: a.topics.Sound, Payload: p, Once: true})
	}
	return msgs
}

func (a *countdownApp) hide(topic string) []Message {
	a.visible = false
	return remove(topic)
}
