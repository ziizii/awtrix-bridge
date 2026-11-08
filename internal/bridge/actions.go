package bridge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// Actions the admin UI can take. Immediate effects (switch, play, arm, timer) go
// straight to MQTT; settings (melodies, alert) are written to the config
// file with a line edit and reloaded, so the file stays the source of
// truth and keeps its comments.

// Switch shows an app on the clock now: a configured app or one of the
// clock's built-in apps.
func (s *Supervisor) Switch(app string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	known := slices.Contains(awtrix.BuiltinApps, app) || app == config.TimerApp ||
		slices.ContainsFunc(inst.cfg.Apps, func(a config.App) bool { return a.Name == app })
	if !known {
		return fmt.Errorf("cannot switch to %q", app)
	}
	return inst.Publish(inst.topics.Switch, awtrix.SwitchPayload(app), false)
}

// PlayMelody plays a built-in melody on the clock now (preview).
func (s *Supervisor) PlayMelody(name string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	tune, ok := config.MelodyRTTTL(name)
	if !ok {
		return fmt.Errorf("unknown melody %q", name)
	}
	return inst.Publish(inst.topics.Sound, awtrix.SoundPayload(tune), false)
}

// SetArmed arms or disarms an app by publishing its retained arm state;
// the bridge's own subscription then applies it, so MQTT stays the single
// source of truth (and anything else, e.g. HA, can arm it the same way).
func (s *Supervisor) SetArmed(app string, on bool) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	topic, ok := inst.eng.ArmTopics()[app]
	if !ok {
		return fmt.Errorf("app %q does not use require_arm", app)
	}
	return inst.Publish(topic, awtrix.ArmPayload(on), true)
}

// SetChime selects the hourly chime melody, or "off".
func (s *Supervisor) SetChime(ctx context.Context, melody string) error {
	if err := checkMelodyChoice(melody); err != nil {
		return err
	}
	return s.edit(ctx, func(raw []byte) ([]byte, error) {
		return config.SetChime(raw, []config.Field{{Key: "melody", Value: melody}}, "rtttl"), nil
	})
}

// SetAlert sets a countdown app's alert: minutes before arrival and a
// built-in melody, or "off".
func (s *Supervisor) SetAlert(ctx context.Context, app string, minutes int, melody string) error {
	if minutes < 1 || minutes > 120 {
		return fmt.Errorf("alert minutes must be 1-120, got %d", minutes)
	}
	if err := checkMelodyChoice(melody); err != nil {
		return err
	}
	return s.edit(ctx, func(raw []byte) ([]byte, error) {
		return config.SetApp(raw, app, []config.Field{
			{Key: "alert_at", Value: fmt.Sprintf("%dm", minutes)},
			{Key: "alert_melody", Value: melody},
		}, "alert_rtttl")
	})
}

func checkMelodyChoice(name string) error {
	if _, ok := config.MelodyRTTTL(name); !ok && name != config.MelodyOff {
		return fmt.Errorf("unknown melody %q", name)
	}
	return nil
}

// maxTimer is the longest timer the UI may start.
const maxTimer = 24 * time.Hour

// StartTimer starts the timer for a number of seconds, or until a time of
// day ("14:30", the next one to come), replacing a running timer. The
// state goes to the retained timer topic, like arming.
func (s *Supervisor) StartTimer(seconds int, at string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	now := time.Now()
	var end time.Time
	switch {
	case at != "" && seconds != 0:
		return errors.New("give a duration or a time of day, not both")
	case at != "":
		if end, err = nextTimeOfDay(at, now); err != nil {
			return err
		}
	case seconds > 0 && time.Duration(seconds)*time.Second <= maxTimer:
		end = now.Add(time.Duration(seconds) * time.Second)
	default:
		return fmt.Errorf("timer must be 1s to %v, got %ds", maxTimer, seconds)
	}
	back := inst.activeApp()
	if back == config.TimerApp {
		back = inst.eng.Timer().Back // keep where the running timer returns to
	}
	return inst.setTimer(awtrix.Timer{Start: now, End: end, Back: back})
}

// ExtendTimer adds seconds to the running timer; one that already ended
// restarts from now.
func (s *Supervisor) ExtendTimer(seconds int) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	t := inst.eng.Timer()
	if !t.Running() {
		return errors.New("no timer is running")
	}
	if seconds < 1 || time.Duration(seconds)*time.Second > maxTimer {
		return fmt.Errorf("can add 1s to %v, got %ds", maxTimer, seconds)
	}
	now := time.Now()
	if t.End.Before(now) {
		t.Start, t.End = now, now
	}
	t.End = t.End.Add(time.Duration(seconds) * time.Second)
	return inst.setTimer(t)
}

// CancelTimer stops the timer; the clock switches back.
func (s *Supervisor) CancelTimer() error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	return inst.setTimer(awtrix.Timer{})
}

// SetTimerMelody selects the melody played when the timer ends, or "off".
func (s *Supervisor) SetTimerMelody(ctx context.Context, melody string) error {
	if err := checkMelodyChoice(melody); err != nil {
		return err
	}
	return s.edit(ctx, func(raw []byte) ([]byte, error) {
		return config.SetSection(raw, "timer", []config.Field{{Key: "melody", Value: melody}}, "rtttl"), nil
	})
}

func (i *instance) setTimer(t awtrix.Timer) error {
	topic := i.eng.TimerTopic()
	if topic == "" {
		return errors.New("no timer app")
	}
	return i.Publish(topic, awtrix.TimerPayload(t), true)
}

// nextTimeOfDay returns the next time the clock shows "15:04", today or
// tomorrow.
func nextTimeOfDay(hhmm string, now time.Time) (time.Time, error) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, fmt.Errorf("time of day must look like 14:30, got %q", hhmm)
	}
	y, m, d := now.Date()
	at := time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, now.Location())
	if !at.After(now) {
		at = time.Date(y, m, d+1, t.Hour(), t.Minute(), 0, 0, now.Location())
	}
	return at, nil
}

// SetTimerIcon sets the timer's icon to a LaMetric icon id, uploading it
// to the clock first if it is missing there; "" removes the icon.
func (s *Supervisor) SetTimerIcon(ctx context.Context, id string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	if err := inst.checkIcon(ctx, id); err != nil {
		return err
	}
	return s.edit(ctx, func(raw []byte) ([]byte, error) {
		return config.SetSection(raw, "timer", []config.Field{{Key: "icon", Value: strconv.Quote(id)}}), nil
	})
}

// checkIcon accepts "" (no icon) or a LaMetric icon id, which it makes
// sure the clock has.
func (i *instance) checkIcon(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if !awtrix.IconID.MatchString(id) {
		return fmt.Errorf("icon must be a LaMetric icon number, got %q", id)
	}
	if err := i.ensureIcon(ctx, id); err != nil {
		return fmt.Errorf("icon %s: %w", id, err)
	}
	return nil
}
