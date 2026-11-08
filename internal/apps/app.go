// Package apps turns Home Assistant states into clock messages. Each app
// type lives in its own file; Build maps config entries to them.
//
// Apps are only ever called from the engine's goroutine, so they keep
// their runtime state in plain fields.
package apps

import (
	"fmt"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

// App renders clock messages from entity states.
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

// Timed is the timer app: its state comes from the retained timer topic.
type Timed interface {
	App
	TimerTopic() string
	Timer() awtrix.Timer
	SetTimer(t awtrix.Timer)
}

// carrier apps keep runtime state (what already played, where the clock
// was switched) across config reloads; see Carry.
type carrier interface {
	carry(from App)
}

// States gives read access to the latest known entity states.
type States interface {
	Get(entityID string) (ha.State, bool)
}

// Message is one MQTT publish. A nil Payload removes a pushed app. Once
// messages (sounds, switches, arm state) are events: never deduplicated
// and never re-sent on resync.
type Message struct {
	Topic   string
	Payload []byte
	Once    bool
	Retain  bool
}

func show(topic string, payload []byte) Message { return Message{Topic: topic, Payload: payload} }
func remove(topic string) Message               { return Message{Topic: topic} }
func event(topic string, payload []byte) Message {
	return Message{Topic: topic, Payload: payload, Once: true}
}

// Build creates the apps described in the config, in config order:
// device settings first, then the apps list.
func Build(c *config.Config) ([]App, error) {
	t := awtrix.NewTopics(c.Awtrix)
	var out []App
	if b := c.Brightness; b != nil {
		out = append(out, &brightnessApp{cfg: *b, topic: t.Settings})
	}
	if ch := c.Chime; ch != nil && ch.Tune() != "" {
		out = append(out, &chimeApp{cfg: *ch, tune: ch.Tune(), topic: t.Sound})
	}
	if c.Timer != nil {
		out = append(out, &timerApp{cfg: *c.Timer, topics: t})
	}
	for _, a := range c.Apps {
		switch a.Type {
		case config.TypeValue:
			out = append(out, &valueApp{cfg: a, topic: t.App(a.Name)})
		case config.TypeSurplus:
			out = append(out, &surplusApp{cfg: a, topic: t.App(a.Name)})
		case config.TypeCountdown:
			out = append(out, &countdownApp{cfg: a, topics: t})
		default:
			return nil, fmt.Errorf("app %q: unknown type %q", a.Name, a.Type)
		}
	}
	return out, nil
}

// Carry hands runtime state from the apps of a previous run to same-named
// apps of a new one, so a config reload does not replay sounds or switches.
func Carry(from, to []App) {
	old := map[string]App{}
	for _, a := range from {
		old[a.Name()] = a
	}
	for _, a := range to {
		if c, ok := a.(carrier); ok && old[a.Name()] != nil {
			c.carry(old[a.Name()])
		}
	}
}
