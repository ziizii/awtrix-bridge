package apps

import (
	"fmt"
	"math"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// timerApp shows the timer started from the admin UI (or by anything
// that writes the timer topic). It switches the clock to itself when the
// timer starts, counts down with a progress bar, plays the melody at the
// end, counts up in red for hold_after, then clears the timer and
// switches back to the app that was on screen before.
type timerApp struct {
	cfg    config.Timer
	topics awtrix.Topics
	st     timerState
}

type timerState struct {
	t        awtrix.Timer
	switched bool      // the clock was switched to the timer
	back     string    // app to switch back to
	rang     time.Time // end the melody played for
}

// A timer seen after a restart does not switch the clock again once it
// is older than timerFresh, and does not ring once it ended more than
// timerLate ago.
const (
	timerFresh = 10 * time.Second
	timerLate  = time.Minute
)

func (a *timerApp) Name() string       { return config.TimerApp }
func (a *timerApp) Entities() []string { return nil }
func (a *timerApp) Ticking() bool      { return true }
func (a *timerApp) topic() string      { return a.topics.App(config.TimerApp) }

func (a *timerApp) TimerTopic() string  { return a.topics.Timer }
func (a *timerApp) Timer() awtrix.Timer { return a.st.t }

func (a *timerApp) SetTimer(t awtrix.Timer) {
	if t.Back != "" {
		a.st.back = t.Back
	}
	a.st.t = t
}

func (a *timerApp) Render(_ States, now time.Time) []Message {
	t := a.st.t
	if !t.Running() {
		return append(a.switchBack(), remove(a.topic()))
	}
	left := t.End.Sub(now)
	if left < -a.cfg.HoldAfter.D() {
		// Done: clear the retained state, which also comes back to us.
		a.st.t = awtrix.Timer{}
		clear := event(a.topics.Timer, awtrix.TimerPayload(awtrix.Timer{}))
		clear.Retain = true
		return append(a.switchBack(), remove(a.topic()), clear)
	}

	msgs := []Message{show(a.topic(), a.payload(t, now))}
	if !a.st.switched && now.Sub(t.Start) < timerFresh {
		a.st.switched = true
		msgs = append(msgs, event(a.topics.Switch, awtrix.SwitchPayload(config.TimerApp)))
	}
	if left <= 0 && !a.st.rang.Equal(t.End) {
		a.st.rang = t.End
		if tune := a.cfg.Tune(); tune != "" && -left < timerLate {
			msgs = append(msgs, event(a.topics.Sound, awtrix.SoundPayload(tune)))
		}
	}
	return msgs
}

// payload shows the time left with a progress bar, or after the end the
// time since in the done color.
func (a *timerApp) payload(t awtrix.Timer, now time.Time) []byte {
	secs := int(math.Ceil(t.End.Sub(now).Seconds()))
	if secs <= 0 {
		return awtrix.AppPayload(a.cfg.Icon, countdownText(secs), a.cfg.DoneColor, nil)
	}
	extra := map[string]any{}
	if total := t.End.Sub(t.Start); total > 0 {
		p := int(100 * now.Sub(t.Start) / total)
		// awtrixng rejects a whole payload with unknown keys, and knows
		// only these two progress keys.
		extra["progress"] = min(100, max(0, p))
		extra["progressColor"] = a.cfg.Color
	}
	return awtrix.AppPayload(a.cfg.Icon, timerText(secs), a.cfg.Color, extra)
}

// timerText renders "4:05", or "1:04:05" from an hour up.
func timerText(secs int) string {
	if secs >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs/60%60, secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// switchBack returns the clock to where it was before the timer, once.
func (a *timerApp) switchBack() []Message {
	if !a.st.switched {
		return nil
	}
	a.st.switched = false
	back := or(a.cfg.SwitchBack, or(a.st.back, "Time"))
	if back == config.TimerApp {
		back = "Time"
	}
	return []Message{event(a.topics.Switch, awtrix.SwitchPayload(back))}
}

func (a *timerApp) carry(from App) {
	if o, ok := from.(*timerApp); ok {
		a.st = o.st
	}
}
