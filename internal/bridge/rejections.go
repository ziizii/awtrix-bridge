package bridge

import (
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
)

// Rejection is a command the clock refused, e.g. an app payload with a
// key its firmware does not know (it then ignores the whole payload).
type Rejection struct {
	Topic  string    `json:"topic"`  // the command topic
	Reason string    `json:"reason"` // the clock's error message
	Count  int       `json:"count"`  // times in a row with this reason
	At     time.Time `json:"at"`     // last time
}

// rejections tracks the clock's answers on the result topics. Each topic
// is logged once when it starts failing (or the reason changes) and once
// when it is accepted again; an app re-published every second would
// otherwise flood the log.
type rejections struct {
	mu  sync.Mutex
	by  map[string]*Rejection // command topic -> current rejection
	log *slog.Logger
}

func newRejections(log *slog.Logger) *rejections {
	return &rejections{by: map[string]*Rejection{}, log: log}
}

func (r *rejections) note(resultTopic string, payload []byte, now time.Time) {
	res, ok := awtrix.ParseResult(payload)
	if !ok {
		return
	}
	topic := awtrix.CommandOf(resultTopic)
	r.mu.Lock()
	defer r.mu.Unlock()
	prev := r.by[topic]
	if res.OK {
		if prev != nil {
			r.log.Info("clock accepts command again", "topic", topic)
			delete(r.by, topic)
		}
		return
	}
	reason := res.Reason()
	if prev == nil || prev.Reason != reason {
		r.log.Warn("clock rejected command", "topic", topic, "reason", reason)
		prev = &Rejection{Topic: topic, Reason: reason}
		r.by[topic] = prev
	}
	prev.Count++
	prev.At = now
}

// list returns the current rejections, by topic.
func (r *rejections) list() []Rejection {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Rejection{}
	for _, v := range r.by {
		out = append(out, *v)
	}
	slices.SortFunc(out, func(a, b Rejection) int { return strings.Compare(a.Topic, b.Topic) })
	return out
}
