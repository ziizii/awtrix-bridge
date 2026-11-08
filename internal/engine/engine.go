// Package engine wires HA state events to app renders and MQTT publishes.
package engine

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

type Publisher interface {
	Publish(topic string, payload []byte, retain bool) error
}

type store map[string]ha.State

func (s store) Get(id string) (ha.State, bool) { v, ok := s[id]; return v, ok }

type Engine struct {
	apps    []apps.App
	byID    map[string][]apps.App
	pub     Publisher
	resync  time.Duration
	log     *slog.Logger
	states  store
	last    map[string][]byte // topic -> last payload published
	ready   bool              // first snapshot received
	resyncC chan struct{}
	armC    chan armReq

	mu     sync.Mutex
	shown  map[string]Published // topic -> what the clock currently has
	recent []Published          // latest one-off messages (switches, sounds), newest last
	order  map[string]int       // app name -> config position
	armed  map[string]bool      // armable app name -> armed
}

type armReq struct {
	name string
	on   bool
}

// ArmState is the arm state of an app that needs arming.
type ArmState struct {
	App   string `json:"app"`
	Topic string `json:"topic"`
	Armed bool   `json:"armed"`
}

// Published is a message the engine sent, for status display.
type Published struct {
	App     string    `json:"app"`
	Topic   string    `json:"topic"`
	Payload string    `json:"payload"` // "" = app removed
	At      time.Time `json:"at"`
}

const recentMax = 20

func New(list []apps.App, pub Publisher, resync time.Duration, log *slog.Logger) *Engine {
	e := &Engine{
		apps: list, byID: map[string][]apps.App{}, pub: pub, resync: resync, log: log,
		states: store{}, last: map[string][]byte{}, resyncC: make(chan struct{}, 1),
		shown: map[string]Published{}, order: map[string]int{}, armed: map[string]bool{},
		armC: make(chan armReq, 16),
	}
	for i, a := range list {
		e.order[a.Name()] = i
		if ar, ok := a.(apps.Armable); ok && ar.ArmTopic() != "" {
			e.armed[a.Name()] = false
		}
		for _, id := range a.Entities() {
			e.byID[id] = append(e.byID[id], a)
		}
	}
	return e
}

// ForceResync asks the engine to republish everything (e.g. after an MQTT reconnect).
func (e *Engine) ForceResync() {
	select {
	case e.resyncC <- struct{}{}:
	default:
	}
}

// SetArmed arms or disarms the named app. Safe to call from any goroutine.
func (e *Engine) SetArmed(name string, on bool) {
	select {
	case e.armC <- armReq{name, on}:
	default:
		e.log.Warn("arm request dropped", "app", name)
	}
}

// ArmTopics maps each app that needs arming to its arm state topic.
func (e *Engine) ArmTopics() map[string]string {
	out := map[string]string{}
	for _, a := range e.apps {
		if ar, ok := a.(apps.Armable); ok && ar.ArmTopic() != "" {
			out[a.Name()] = ar.ArmTopic()
		}
	}
	return out
}

// Run is the single goroutine that owns all engine state.
func (e *Engine) Run(ctx context.Context, events <-chan ha.Event) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	resync := time.NewTicker(e.resync)
	defer resync.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			e.handle(ev, time.Now())
		case now := <-tick.C:
			if e.ready {
				for _, a := range e.apps {
					if a.Ticking() {
						e.render(a, now)
					}
				}
			}
		case <-resync.C:
			e.republish()
		case <-e.resyncC:
			e.republish()
		case r := <-e.armC:
			for _, a := range e.apps {
				if ar, ok := a.(apps.Armable); ok && a.Name() == r.name && ar.ArmTopic() != "" && ar.Armed() != r.on {
					ar.SetArmed(r.on, time.Now())
					e.log.Info("arm state changed", "app", r.name, "armed", r.on)
					if e.ready {
						e.render(a, time.Now())
					} else {
						e.syncArmed(a)
					}
				}
			}
		}
	}
}

func (e *Engine) handle(ev ha.Event, now time.Time) {
	if ev.Snapshot {
		for _, s := range ev.States {
			if _, watched := e.byID[s.EntityID]; watched {
				e.states[s.EntityID] = s
			}
		}
		e.ready = true
		e.log.Info("state snapshot received", "watched_entities", len(e.byID))
		for _, a := range e.apps {
			e.render(a, now)
		}
		return
	}
	if !e.ready {
		return
	}
	dirty := map[apps.App]bool{}
	for _, s := range ev.States {
		deps, ok := e.byID[s.EntityID]
		if !ok {
			continue
		}
		e.states[s.EntityID] = s
		for _, a := range deps {
			dirty[a] = true
		}
	}
	for _, a := range e.apps { // keep config order
		if dirty[a] {
			e.render(a, now)
		}
	}
}

func (e *Engine) render(a apps.App, now time.Time) {
	defer e.syncArmed(a)
	for _, m := range a.Render(e.states, now) {
		if !m.Once {
			if prev, seen := e.last[m.Topic]; seen && bytes.Equal(prev, m.Payload) {
				continue
			}
		}
		if err := e.pub.Publish(m.Topic, m.Payload, m.Retain); err != nil {
			e.log.Warn("publish failed", "topic", m.Topic, "err", err)
			delete(e.last, m.Topic) // retry on next render
			continue
		}
		e.log.Debug("published", "app", a.Name(), "topic", m.Topic, "payload", string(m.Payload))
		if !m.Once {
			e.last[m.Topic] = m.Payload
		}
		e.record(Published{App: a.Name(), Topic: m.Topic, Payload: string(m.Payload), At: now}, m.Once)
	}
}

func (e *Engine) syncArmed(a apps.App) {
	ar, ok := a.(apps.Armable)
	if !ok || ar.ArmTopic() == "" {
		return
	}
	e.mu.Lock()
	e.armed[a.Name()] = ar.Armed()
	e.mu.Unlock()
}

// Arms returns the arm state of every app that needs arming, in config order.
func (e *Engine) Arms() []ArmState {
	topics := e.ArmTopics()
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []ArmState
	for name, on := range e.armed {
		out = append(out, ArmState{App: name, Topic: topics[name], Armed: on})
	}
	slices.SortFunc(out, func(a, b ArmState) int { return e.order[a.App] - e.order[b.App] })
	return out
}

func (e *Engine) record(p Published, once bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !once {
		e.shown[p.Topic] = p
		return
	}
	if p.At.IsZero() {
		p.At = time.Now()
	}
	e.recent = append(e.recent, p)
	if len(e.recent) > recentMax {
		e.recent = e.recent[len(e.recent)-recentMax:]
	}
}

// Status returns what was last published per topic (in config order) and
// the latest one-off messages. Safe to call from any goroutine.
func (e *Engine) Status() (shown, recent []Published) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.shown {
		shown = append(shown, p)
	}
	slices.SortFunc(shown, func(a, b Published) int { return e.order[a.App] - e.order[b.App] })
	return shown, slices.Clone(e.recent)
}

func (e *Engine) republish() {
	if !e.ready {
		return
	}
	clear(e.last)
	now := time.Now()
	for _, a := range e.apps {
		e.render(a, now)
	}
}
