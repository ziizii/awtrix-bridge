package bridge

import (
	"context"
	"fmt"
	"slices"

	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
)

// Actions the admin UI can take. Immediate effects (switch, play, arm) go
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
	known := slices.Contains(awtrix.BuiltinApps, app) || slices.ContainsFunc(inst.cfg.Apps, func(a config.App) bool { return a.Name == app })
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
