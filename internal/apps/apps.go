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
	Retain  bool
}

type App interface {
	Name() string
	// Entities lists the entity IDs whose changes trigger a re-render.
	Entities() []string
	// Ticking apps are also re-rendered every second.
	Ticking() bool
	Render(st States, now time.Time) []Message
}

// Armable apps stay hidden until armed and disarm themselves.
type Armable interface {
	App
	ArmTopic() string // "" if the app does not need arming
	Armed() bool
	SetArmed(on bool, now time.Time)
}

type Topics struct {
	App      string // contains {name}
	Settings string
	Sound    string
	Switch   string
	Arm      string // contains {name}
}

func (t Topics) app(name string) string { return strings.ReplaceAll(t.App, "{name}", name) }
func (t Topics) arm(name string) string { return strings.ReplaceAll(t.Arm, "{name}", name) }

// Build creates the apps described in the config.
func Build(c *config.Config) ([]App, error) {
	t := Topics{
		App:      c.Awtrix.AppTopic,
		Settings: c.Awtrix.SettingsTopic,
		Sound:    c.Awtrix.SoundTopic,
		Switch:   SwitchTopic(c.Awtrix.AppTopic),
		Arm:      c.Awtrix.ArmTopic,
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

// SwitchTopic derives ".../apps/switch" from ".../apps/pushed/{name}".
func SwitchTopic(appTopic string) string {
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
// updating every second, then counts up for HoldAfter once it arrives.
type countdownApp struct {
	cfg      config.App
	topics   Topics
	bus      time.Time // latest arrival estimate of the bus being shown
	alerted  time.Time // bus the alert already played for
	switched time.Time // bus the clock was switched to, until it switches back
	armed    bool      // require_arm: currently armed
	armedAt  time.Time // when it was armed; only buses arriving later disarm it
	pickup   time.Time // require_arm: the bus that arrived while armed
}

// sameTarget is how far a live arrival estimate may drift and still count
// as the same bus. Real-time feeds move the timestamp by seconds every
// minute; without this the alert would replay on every update.
const sameTarget = 10 * time.Minute

func (a *countdownApp) Name() string       { return a.cfg.Name }
func (a *countdownApp) Entities() []string { return []string{a.cfg.Entity} }
func (a *countdownApp) Ticking() bool      { return true }

func (a *countdownApp) Render(st States, now time.Time) []Message {
	topic := a.topics.app(a.cfg.Name)
	hold := a.cfg.HoldAfter.D()
	msgs := append(a.switchBack(now, hold), a.autoDisarm(now, hold)...)

	target := a.target(st, now, hold)
	if target.IsZero() || (a.cfg.RequireArm && !a.armed) {
		return append(msgs, remove(topic)...)
	}
	remaining := target.Sub(now)
	if remaining < -hold || (a.cfg.ShowWithin > 0 && remaining > a.cfg.ShowWithin.D()) {
		return append(msgs, remove(topic)...)
	}

	if sameBus(a.pickup, target) || (a.armed && remaining <= 0 && target.After(a.armedAt)) {
		a.pickup = target
	}

	secs := int(math.Ceil(remaining.Seconds()))
	text := fmt.Sprintf("%d:%02d", secs/60, secs%60)
	if secs <= 0 {
		text = fmt.Sprintf("+%d:%02d", -secs/60, -secs%60)
	}
	mins := float64(secs) / 60
	msgs = append(msgs, Message{Topic: topic, Payload: payload(a.cfg.Icon, text, pickColor(mins, a.cfg.Color, a.cfg.Colors), a.cfg.Extra)})

	if sameBus(a.switched, target) {
		a.switched = target // follow the drifting estimate
	} else if a.cfg.SwitchAt > 0 && a.topics.Switch != "" && remaining > 0 && remaining <= a.cfg.SwitchAt.D() {
		a.switched = target
		p, _ := json.Marshal(map[string]any{"name": a.cfg.Name, "fast": true})
		// The switch must follow the app payload so the app exists.
		msgs = append(msgs, Message{Topic: a.topics.Switch, Payload: p, Once: true})
	}

	if a.cfg.AlertAt > 0 && a.cfg.AlertRTTTL != "" && remaining > 0 &&
		remaining <= a.cfg.AlertAt.D() && !sameBus(a.alerted, target) {
		a.alerted = target
		p, _ := json.Marshal(map[string]string{"rtttl": a.cfg.AlertRTTTL})
		msgs = append(msgs, Message{Topic: a.topics.Sound, Payload: p, Once: true})
	}
	return msgs
}

// target returns the arrival to show (zero if unknown). The feed moves on
// to the next bus shortly after one arrives, so the bus that just arrived
// is kept for hold to count up.
func (a *countdownApp) target(st States, now time.Time, hold time.Duration) time.Time {
	var t time.Time
	if s, ok := st.Get(a.cfg.Entity); ok {
		t, _ = time.Parse(time.RFC3339, s.State) // zero if unparsable
	}
	if !a.bus.IsZero() && !sameBus(a.bus, t) && !now.Before(a.bus) && now.Sub(a.bus) <= hold {
		return a.bus
	}
	a.bus = t
	return t
}

// switchBack returns the clock to SwitchBack once the bus it was switched
// for has finished counting up.
func (a *countdownApp) switchBack(now time.Time, hold time.Duration) []Message {
	if a.switched.IsZero() || now.Sub(a.switched) <= hold {
		return nil
	}
	a.switched = time.Time{}
	if a.cfg.SwitchBack == "" || a.topics.Switch == "" {
		return nil
	}
	p, _ := json.Marshal(map[string]any{"name": a.cfg.SwitchBack, "fast": true})
	return []Message{{Topic: a.topics.Switch, Payload: p, Once: true}}
}

// autoDisarm disarms a require_arm app once the bus that arrived while it
// was armed has finished counting up, and publishes the new state.
func (a *countdownApp) autoDisarm(now time.Time, hold time.Duration) []Message {
	if a.pickup.IsZero() || now.Sub(a.pickup) <= hold {
		return nil
	}
	a.armed, a.pickup = false, time.Time{}
	if a.ArmTopic() == "" {
		return nil
	}
	return []Message{{Topic: a.ArmTopic(), Payload: []byte("off"), Once: true, Retain: true}}
}

func (a *countdownApp) ArmTopic() string {
	if !a.cfg.RequireArm || a.topics.Arm == "" {
		return ""
	}
	return a.topics.arm(a.cfg.Name)
}

func (a *countdownApp) Armed() bool { return a.armed }

func (a *countdownApp) SetArmed(on bool, now time.Time) {
	if on == a.armed {
		return
	}
	a.armed, a.pickup = on, time.Time{}
	if on {
		a.armedAt = now
	}
}

// sameBus reports whether target is the bus already handled at prev.
func sameBus(prev, target time.Time) bool {
	if prev.IsZero() || target.IsZero() {
		return false
	}
	d := target.Sub(prev)
	return d > -sameTarget && d < sameTarget
}

// Carry copies per-bus state (alert played, clock switched, armed) from the apps
// of a previous run to same-named apps of a new one, so a config reload
// does not replay sounds or switches.
func Carry(from, to []App) {
	old := map[string]*countdownApp{}
	for _, a := range from {
		if c, ok := a.(*countdownApp); ok {
			old[c.cfg.Name] = c
		}
	}
	for _, a := range to {
		if c, ok := a.(*countdownApp); ok {
			if o := old[c.cfg.Name]; o != nil {
				c.bus, c.alerted, c.switched = o.bus, o.alerted, o.switched
				c.armed, c.armedAt, c.pickup = o.armed, o.armedAt, o.pickup
			}
		}
	}
}
