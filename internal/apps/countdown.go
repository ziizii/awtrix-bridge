package apps

import (
	"fmt"
	"math"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// countdownApp counts down to a timestamp entity (e.g. the next bus
// arrival) every second, then counts up for hold_after once it arrives.
// Around that it can switch the clock to itself (switch_at) and back
// (switch_back), play an alert (alert_at), and with require_arm stay
// hidden until armed and disarm itself after the next arrival.
type countdownApp struct {
	cfg    config.App
	topics awtrix.Topics
	st     countdownState
	colorPreview
}

// countdownState is the runtime state kept across renders and reloads.
// Buses are identified by their (drifting) arrival estimate; see sameBus.
type countdownState struct {
	bus      time.Time // latest arrival estimate of the bus being shown
	alerted  time.Time // bus the alert played for (apps without require_arm)
	switched time.Time // bus the clock was switched to, until it switches back
	arm      armState
}

type armState struct {
	on      bool
	since   time.Time // only buses arriving after this disarm it
	pickup  time.Time // the bus that arrived while armed
	alerted bool      // the alert played since arming
}

// sameTarget is how far a live arrival estimate may drift and still count
// as the same bus. Real-time feeds move the timestamp by seconds every
// minute and reset it to the schedule every hour.
const sameTarget = 10 * time.Minute

// sameBus reports whether target is the bus already handled at prev.
func sameBus(prev, target time.Time) bool {
	if prev.IsZero() || target.IsZero() {
		return false
	}
	d := target.Sub(prev)
	return d > -sameTarget && d < sameTarget
}

func (a *countdownApp) Name() string       { return a.cfg.Name }
func (a *countdownApp) Entities() []string { return []string{a.cfg.Entity} }
func (a *countdownApp) Ticking() bool      { return true }
func (a *countdownApp) topic() string      { return a.topics.App(a.cfg.Name) }

func (a *countdownApp) Render(st States, now time.Time) []Message {
	hold := a.cfg.HoldAfter.D()
	msgs := append(a.switchBack(now, hold), a.autoDisarm(now, hold)...)

	target := a.target(st, now, hold)
	remaining := target.Sub(now)
	if !a.visible(target, remaining, hold) {
		return append(msgs, remove(a.topic()))
	}
	a.trackPickup(target, remaining)
	msgs = append(msgs, show(a.topic(), a.payload(remaining)))
	msgs = append(msgs, a.switchTo(target, remaining)...) // after the payload, so the app exists
	return append(msgs, a.alert(target, remaining)...)
}

// target returns the arrival to show (zero if unknown). The feed moves on
// to the next bus ~20s after one arrives, so the bus that just arrived is
// kept for hold to count up.
func (a *countdownApp) target(st States, now time.Time, hold time.Duration) time.Time {
	var t time.Time
	if s, ok := st.Get(a.cfg.Entity); ok {
		t, _ = time.Parse(time.RFC3339, s.State) // zero if unparsable
	}
	prev := a.st.bus
	if !prev.IsZero() && !sameBus(prev, t) && !now.Before(prev) && now.Sub(prev) <= hold {
		return prev
	}
	a.st.bus = t
	return t
}

func (a *countdownApp) visible(target time.Time, remaining, hold time.Duration) bool {
	switch {
	case target.IsZero(), a.cfg.RequireArm && !a.st.arm.on:
		return false
	case remaining < -hold:
		return false
	case a.cfg.ShowWithin > 0 && remaining > a.cfg.ShowWithin.D():
		return false
	}
	return true
}

func (a *countdownApp) payload(remaining time.Duration) []byte {
	secs := int(math.Ceil(remaining.Seconds()))
	color := pickColor(float64(secs)/60, a.col("color", a.cfg.Color), a.thresholds(a.cfg.Colors))
	return awtrix.AppPayload(a.cfg.Icon, countdownText(secs), color, a.cfg.Extra)
}

// countdownText renders "4:05" before arrival and "+0:42" after it.
func countdownText(secs int) string {
	if secs <= 0 {
		return fmt.Sprintf("+%d:%02d", -secs/60, -secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// switchTo switches the clock to this app once per bus when it drops
// below switch_at.
func (a *countdownApp) switchTo(target time.Time, remaining time.Duration) []Message {
	if sameBus(a.st.switched, target) {
		a.st.switched = target // follow the drifting estimate for switchBack
		return nil
	}
	if a.cfg.SwitchAt <= 0 || remaining <= 0 || remaining > a.cfg.SwitchAt.D() {
		return nil
	}
	a.st.switched = target
	return []Message{event(a.topics.Switch, awtrix.SwitchPayload(a.cfg.Name))}
}

// switchBack returns the clock to switch_back once the bus it was
// switched for has finished counting up.
func (a *countdownApp) switchBack(now time.Time, hold time.Duration) []Message {
	if a.st.switched.IsZero() || now.Sub(a.st.switched) <= hold {
		return nil
	}
	a.st.switched = time.Time{}
	if a.cfg.SwitchBack == "" {
		return nil
	}
	return []Message{event(a.topics.Switch, awtrix.SwitchPayload(a.cfg.SwitchBack))}
}

// alert plays the alert tune when remaining drops below alert_at.
func (a *countdownApp) alert(target time.Time, remaining time.Duration) []Message {
	tune := a.cfg.AlertTune()
	if tune == "" || a.cfg.AlertAt <= 0 || remaining <= 0 || remaining > a.cfg.AlertAt.D() || !a.mayAlert(target) {
		return nil
	}
	return []Message{event(a.topics.Sound, awtrix.SoundPayload(tune))}
}

// mayAlert reports whether the alert may play for target, and records it.
// An armed app alerts once per arming: a delay can push the arrival back
// out of the alert window and in again, which must not beep twice.
// Otherwise it alerts once per bus.
func (a *countdownApp) mayAlert(target time.Time) bool {
	if a.cfg.RequireArm {
		if a.st.arm.alerted {
			return false
		}
		a.st.arm.alerted = true
		return true
	}
	if sameBus(a.st.alerted, target) {
		return false
	}
	a.st.alerted = target
	return true
}

// ---- arming ------------------------------------------------------------

func (a *countdownApp) ArmTopic() string {
	if !a.cfg.RequireArm {
		return ""
	}
	return a.topics.Arm(a.cfg.Name)
}

func (a *countdownApp) Armed() bool { return a.st.arm.on }

func (a *countdownApp) SetArmed(on bool, now time.Time) {
	if on == a.st.arm.on {
		return
	}
	a.st.arm = armState{on: on}
	if on {
		a.st.arm.since = now
	}
}

// trackPickup remembers the bus that arrives while armed (only buses
// arriving after arming count) and follows its drifting estimate.
func (a *countdownApp) trackPickup(target time.Time, remaining time.Duration) {
	arm := &a.st.arm
	if sameBus(arm.pickup, target) || (arm.on && remaining <= 0 && target.After(arm.since)) {
		arm.pickup = target
	}
}

// autoDisarm disarms once the picked-up bus has finished counting up, and
// publishes the new retained state.
func (a *countdownApp) autoDisarm(now time.Time, hold time.Duration) []Message {
	arm := &a.st.arm
	if arm.pickup.IsZero() || now.Sub(arm.pickup) <= hold {
		return nil
	}
	*arm = armState{}
	if a.ArmTopic() == "" {
		return nil
	}
	m := event(a.ArmTopic(), awtrix.ArmPayload(false))
	m.Retain = true
	return []Message{m}
}

func (a *countdownApp) carry(from App) {
	if o, ok := from.(*countdownApp); ok {
		a.st = o.st
	}
}
