// Package engine wires HA state events to app renders and MQTT publishes.
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
	Publish(topic string, payload []byte) error
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
}

func New(list []apps.App, pub Publisher, resync time.Duration, log *slog.Logger) *Engine {
	e := &Engine{
		apps: list, byID: map[string][]apps.App{}, pub: pub, resync: resync, log: log,
		states: store{}, last: map[string][]byte{}, resyncC: make(chan struct{}, 1),
	}
	for _, a := range list {
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
	for _, m := range a.Render(e.states, now) {
		if !m.Once {
			if prev, seen := e.last[m.Topic]; seen && bytes.Equal(prev, m.Payload) {
				continue
			}
		}
		if err := e.pub.Publish(m.Topic, m.Payload); err != nil {
			e.log.Warn("publish failed", "topic", m.Topic, "err", err)
			delete(e.last, m.Topic) // retry on next render
			continue
		}
		e.log.Debug("published", "app", a.Name(), "topic", m.Topic, "payload", string(m.Payload))
		if !m.Once {
			e.last[m.Topic] = m.Payload
		}
	}
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
