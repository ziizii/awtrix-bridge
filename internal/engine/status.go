package engine

import (
	"slices"
	"sync"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

// Published is a message the engine sent, for status display.
type Published struct {
	App     string    `json:"app"`
	Topic   string    `json:"topic"`
	Payload string    `json:"payload"` // "" = app removed
	At      time.Time `json:"at"`
}

const recentMax = 20

// status mirrors what the Run goroutine knows, for readers on other
// goroutines (the admin UI). The Run goroutine writes it; everything else
// only reads through the exported methods below.
type status struct {
	mu     sync.Mutex
	order  map[string]int       // app name -> config position
	states map[string]ha.State  // latest watched entity states
	shown  map[string]Published // topic -> what the clock currently has
	recent []Published          // latest events (switches, sounds), newest last
	armed  map[string]bool      // armable app -> armed
	timer  awtrix.Timer         // the timer app's state
}

func newStatus(list []apps.App) *status {
	s := &status{order: map[string]int{}, states: map[string]ha.State{}, shown: map[string]Published{}, armed: map[string]bool{}}
	for i, a := range list {
		s.order[a.Name()] = i
		if ar, ok := a.(apps.Armable); ok && ar.ArmTopic() != "" {
			s.armed[a.Name()] = false
		}
	}
	return s
}

func (s *status) setState(st ha.State) {
	s.mu.Lock()
	s.states[st.EntityID] = st
	s.mu.Unlock()
}

func (s *status) record(app string, m apps.Message, at time.Time) {
	p := Published{App: app, Topic: m.Topic, Payload: string(m.Payload), At: at}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !m.Once {
		s.shown[p.Topic] = p
		return
	}
	s.recent = append(s.recent, p)
	if len(s.recent) > recentMax {
		s.recent = s.recent[len(s.recent)-recentMax:]
	}
}

// sync mirrors an app's arm and timer state after it changed.
func (s *status) sync(a apps.App) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ar, ok := a.(apps.Armable); ok && ar.ArmTopic() != "" {
		s.armed[a.Name()] = ar.Armed()
	}
	if t, ok := a.(apps.Timed); ok {
		s.timer = t.Timer()
	}
}

func (s *status) byOrder(a, b string) int { return s.order[a] - s.order[b] }

// State returns the latest known state of a watched entity.
func (e *Engine) State(id string) (ha.State, bool) {
	s := e.status
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[id]
	return st, ok
}

// Shown returns what was last published per topic, in config order.
func (e *Engine) Shown() []Published {
	s := e.status
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Published, 0, len(s.shown))
	for _, p := range s.shown {
		out = append(out, p)
	}
	slices.SortStableFunc(out, func(a, b Published) int { return s.byOrder(a.App, b.App) })
	return out
}

// Recent returns the latest events (switches, sounds, arm changes), oldest first.
func (e *Engine) Recent() []Published {
	s := e.status
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Published{}, s.recent...) // never nil: the UI gets []
}

// Armed reports whether an app that needs arming is armed.
func (e *Engine) Armed(app string) bool {
	s := e.status
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.armed[app]
}

// Timer returns the timer's state (zero when none runs).
func (e *Engine) Timer() awtrix.Timer {
	s := e.status
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.timer
}
