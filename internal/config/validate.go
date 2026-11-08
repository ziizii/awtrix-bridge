package config

import (
	"errors"
	"fmt"
	"time"
)

func (c *Config) applyDefaults() {
	setDefault(&c.MQTT.ClientID, "awtrix-bridge")
	a := &c.Awtrix
	setDefault(&a.AppTopic, "awtrix/cmd/apps/pushed/{name}")
	setDefault(&a.SettingsTopic, "awtrix/cmd/settings")
	setDefault(&a.SoundTopic, "awtrix/cmd/audio/play")
	setDefault(&a.SwitchTopic, "awtrix/cmd/apps/switch")
	setDefault(&a.ActiveTopic, "awtrix/state/apps/active")
	setDefault(&a.ClockSettingsTopic, "awtrix/state/settings")
	setDefault(&a.ArmTopic, "awtrix-bridge/{name}/armed")
	setDefault(&a.TimerTopic, "awtrix-bridge/timer")
	if c.Timer == nil {
		c.Timer = &Timer{}
	}
	t := c.Timer
	if t.Melody == "" && t.RTTTL == "" {
		t.Melody = "beeps"
	}
	setDefault(&t.Color, "#FFFFFF")
	setDefault(&t.DoneColor, "#FF3B30")
	if t.HoldAfter == 0 {
		t.HoldAfter = Duration(time.Minute)
	}
	if a.ResyncInterval == 0 {
		a.ResyncInterval = Duration(5 * time.Minute)
	}
	for i := range c.Apps {
		if c.Apps[i].Scale == 0 {
			c.Apps[i].Scale = 1
		}
	}
}

func setDefault(s *string, def string) {
	if *s == "" {
		*s = def
	}
}

func (c *Config) validate() error {
	if c.HomeAssistant.URL == "" || c.HomeAssistant.Token == "" {
		return errors.New("homeassistant.url and homeassistant.token are required")
	}
	if c.MQTT.Broker == "" {
		return errors.New("mqtt.broker is required")
	}
	if c.Chime != nil {
		if err := checkMelody(c.Chime.Melody); err != nil {
			return fmt.Errorf("chime.melody: %w", err)
		}
	}
	if err := checkMelody(c.Timer.Melody); err != nil {
		return fmt.Errorf("timer.melody: %w", err)
	}
	seen := map[string]bool{TimerApp: true}
	for _, a := range c.Apps {
		if a.Name == "" {
			return errors.New("app without name")
		}
		if seen[a.Name] {
			if a.Name == TimerApp {
				return fmt.Errorf("app name %q is taken by the built-in timer", a.Name)
			}
			return fmt.Errorf("duplicate app name %q", a.Name)
		}
		seen[a.Name] = true
		if err := a.validate(); err != nil {
			return fmt.Errorf("app %q: %w", a.Name, err)
		}
	}
	return nil
}

func (a App) validate() error {
	switch a.Type {
	case TypeValue:
		if a.Entity == "" {
			return errors.New("entity is required")
		}
	case TypeCountdown:
		if a.Entity == "" {
			return errors.New("entity is required")
		}
		if err := checkMelody(a.AlertMelody); err != nil {
			return fmt.Errorf("alert_melody: %w", err)
		}
	case TypeSurplus:
		if a.Production == "" || a.Consumption == "" {
			return errors.New("production and consumption are required")
		}
	default:
		return fmt.Errorf("unknown type %q", a.Type)
	}
	return nil
}
