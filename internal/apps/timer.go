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
	colorPreview
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
// timerTrack is the unfilled part of the progress bar: dim, so the
// filled part shows in any color.
const timerTrack = "#303030"

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
		if a.previewing() { // a sample, so colors can be picked without a timer
			return []Message{show(a.topic(), a.sample(now))}
		}
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

// sample is what the timer looks like while colors are picked and no
// timer runs: 2:05 left of 5:00, or 12s after the end while the done
// color is picked.
func (a *timerApp) sample(now time.Time) []byte {
	t := awtrix.Timer{Start: now.Add(-175 * time.Second), End: now.Add(125 * time.Second)}
	if a.lastKey == "done_color" {
		t = awtrix.Timer{Start: now.Add(-5 * time.Minute), End: now.Add(-12 * time.Second)}
	}
	return a.payload(t, now)
}

// payload shows the time left with a progress bar, or after the end the
// time since in the done color.
func (a *timerApp) payload(t awtrix.Timer, now time.Time) []byte {
	secs := int(math.Ceil(t.End.Sub(now).Seconds()))
	if secs <= 0 {
		return awtrix.AppPayload(a.cfg.Icon, countdownText(secs), a.col("done_color", a.cfg.DoneColor), nil)
	}
	// awtrixng's progress keys (it rejects a payload with unknown ones).
	// The track is white unless set, which hides a white bar.
	extra := map[string]any{
		"progress":           barProgress(now.Sub(t.Start), t.End.Sub(t.Start)),
		"progressColor":      a.col("progress_color", or(a.cfg.ProgressColor, a.cfg.Color)),
		"progressTrackColor": a.col("track_color", or(a.cfg.TrackColor, timerTrack)),
	}
	return awtrix.AppPayload(a.cfg.Icon, timerText(secs), a.col("color", a.cfg.Color), extra)
}

// barPixels is the width of the clock's progress bar: its whole bottom
// row, with or without an icon. It lights floor(progress*32/100) pixels.
const barPixels = 32

// barProgress paces the bar like Bresenham's line: pixel k lights exactly
// k/32 of the way through (integer math, no float drift), and the percent
// sent is the smallest one the clock maps to that many pixels. Sending
// the plain percentage instead lights pixels unevenly, since 100 is not a
// multiple of 32.
func barProgress(elapsed, total time.Duration) int {
	if total <= 0 {
		return 100
	}
	lit := min(barPixels, int(barPixels*int64(max(0, elapsed))/int64(total)))
	return (lit*100 + barPixels - 1) / barPixels
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
