// Package engine runs the apps: it feeds them Home Assistant states, ticks
// them every second, and publishes what they render, skipping payloads the
// clock already has.
//
// All app state is owned by the single Run goroutine. Other goroutines
// change it only through exec (see SetArmed) and read the mirror in
// status.go.
package engine

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

type Publisher interface {
	Publish(topic string, payload []byte, retain bool) error
}

type Engine struct {
	apps   []apps.App
	byID   map[string][]apps.App // entity -> apps that render it
	pub    Publisher
	resync time.Duration
	log    *slog.Logger

	states  store
	last    map[string][]byte // topic -> last payload published
	ready   bool              // first snapshot received
	resyncC chan struct{}
	cmds    chan func(now time.Time)

	status *status
}

type store map[string]ha.State

func (s store) Get(id string) (ha.State, bool) { v, ok := s[id]; return v, ok }

func New(list []apps.App, pub Publisher, resync time.Duration, log *slog.Logger) *Engine {
	e := &Engine{
		apps: list, byID: map[string][]apps.App{}, pub: pub, resync: resync, log: log,
		states: store{}, last: map[string][]byte{},
		resyncC: make(chan struct{}, 1), cmds: make(chan func(time.Time), 16),
		status: newStatus(list),
	}
	for _, a := range list {
		for _, id := range a.Entities() {
			e.byID[id] = append(e.byID[id], a)
		}
	}
	return e
}

// ForceResync asks the engine to republish everything (e.g. after an MQTT
// reconnect). Safe to call from any goroutine.
func (e *Engine) ForceResync() {
	select {
	case e.resyncC <- struct{}{}:
	default:
	}
}

// exec runs f on the engine goroutine. It never blocks the caller (MQTT
// handlers call it); if the queue is full the command is dropped.
func (e *Engine) exec(name string, f func(now time.Time)) {
	select {
	case e.cmds <- f:
	default:
		e.log.Warn("engine busy, command dropped", "cmd", name)
	}
}

// Run is the single goroutine that owns all app state.
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
			e.tick(now)
		case <-resync.C:
			e.republish()
		case <-e.resyncC:
			e.republish()
		case f := <-e.cmds:
			f(time.Now())
		}
	}
}

func (e *Engine) handle(ev ha.Event, now time.Time) {
	if ev.Snapshot {
		for _, s := range ev.States {
			if _, watched := e.byID[s.EntityID]; watched {
				e.setState(s)
			}
		}
		e.ready = true
		e.log.Info("state snapshot received", "watched_entities", len(e.byID))
		e.renderAll(now)
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
		e.setState(s)
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

func (e *Engine) tick(now time.Time) {
	if !e.ready {
		return
	}
	for _, a := range e.apps {
		if a.Ticking() {
			e.render(a, now)
		}
	}
}

// republish forgets what the clock has and sends everything again.
func (e *Engine) republish() {
	if !e.ready {
		return
	}
	clear(e.last)
	e.renderAll(time.Now())
}

func (e *Engine) renderAll(now time.Time) {
	for _, a := range e.apps {
		e.render(a, now)
	}
}

func (e *Engine) render(a apps.App, now time.Time) {
	defer e.status.syncArmed(a)
	for _, m := range a.Render(e.states, now) {
		if prev, seen := e.last[m.Topic]; seen && !m.Once && bytes.Equal(prev, m.Payload) {
			continue // the clock already has it
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
		e.status.record(a.Name(), m, now)
	}
}

func (e *Engine) setState(s ha.State) {
	e.states[s.EntityID] = s
	e.status.setState(s)
}

// SetArmed arms or disarms the named app. Safe to call from any goroutine.
func (e *Engine) SetArmed(name string, on bool) {
	e.exec("arm", func(now time.Time) {
		for _, a := range e.apps {
			ar, ok := a.(apps.Armable)
			if !ok || a.Name() != name || ar.ArmTopic() == "" || ar.Armed() == on {
				continue
			}
			ar.SetArmed(on, now)
			e.log.Info("arm state changed", "app", name, "armed", on)
			if e.ready {
				e.render(a, now)
			} else {
				e.status.syncArmed(a)
			}
		}
	})
}

// ArmTopics maps each app that needs arming to its arm state topic. It
// only reads configuration, so it is safe from any goroutine.
func (e *Engine) ArmTopics() map[string]string {
	out := map[string]string{}
	for _, a := range e.apps {
		if ar, ok := a.(apps.Armable); ok && ar.ArmTopic() != "" {
			out[a.Name()] = ar.ArmTopic()
		}
	}
	return out
}
