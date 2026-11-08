// Package config loads the YAML configuration for awtrix-bridge.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from strings like "90s" or "30m".
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: invalid duration %q: %w", n.Line, n.Value, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

type Config struct {
	HomeAssistant HomeAssistant `yaml:"homeassistant"`
	MQTT          MQTT          `yaml:"mqtt"`
	Awtrix        Awtrix        `yaml:"awtrix"`
	Brightness    *Brightness   `yaml:"brightness"`
	Apps          []App         `yaml:"apps"`
}

type HomeAssistant struct {
	// URL is the HA base URL, e.g. http://homeassistant.local:8123
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type MQTT struct {
	Broker   string `yaml:"broker"` // tcp://host:1883
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	ClientID string `yaml:"client_id"`
}

type Awtrix struct {
	// AppTopic is the topic for a custom app; "{name}" is replaced with the app name.
	AppTopic      string `yaml:"app_topic"`
	SettingsTopic string `yaml:"settings_topic"`
	SoundTopic    string `yaml:"sound_topic"`
	// ArmTopic holds the retained on/off arm state of require_arm apps; "{name}" is replaced.
	ArmTopic string `yaml:"arm_topic"`
	// ResyncInterval re-publishes every app periodically so the clock recovers after a reboot.
	ResyncInterval Duration `yaml:"resync_interval"`
}

type Brightness struct {
	QuietEntity string `yaml:"quiet_entity"` // on => quiet brightness
	Quiet       int    `yaml:"quiet"`
	Normal      int    `yaml:"normal"`
}

// Threshold selects a color: the first entry whose Below is greater than the
// value wins; an entry without Below is the fallback.
type Threshold struct {
	Below *float64 `yaml:"below"`
	Color string   `yaml:"color"`
}

type App struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // value | surplus | countdown

	// value / countdown
	Entity string      `yaml:"entity"`
	Icon   string      `yaml:"icon"`
	Color  string      `yaml:"color"`
	Colors []Threshold `yaml:"colors"`

	// value
	Format string  `yaml:"format"` // fmt verb applied to the numeric state, e.g. "%.0fW"
	Scale  float64 `yaml:"scale"`  // multiply the state before formatting (default 1)

	// surplus
	Production    string `yaml:"production"`
	Consumption   string `yaml:"consumption"`
	NightIcon     string `yaml:"night_icon"` // icon when production is 0
	PositiveColor string `yaml:"positive_color"`
	NegativeColor string `yaml:"negative_color"`

	// countdown: Colors thresholds are in minutes remaining.
	ShowWithin Duration `yaml:"show_within"` // only show when arrival is within this window (0 = always)
	HoldAfter  Duration `yaml:"hold_after"`  // count up ("+0:42") this long after arrival
	AlertAt    Duration `yaml:"alert_at"`    // play a sound once when remaining drops below this
	AlertRTTTL string   `yaml:"alert_rtttl"`
	SwitchAt   Duration `yaml:"switch_at"`   // switch the clock to this app once when remaining drops below this (0 = never)
	SwitchBack string   `yaml:"switch_back"` // app to switch back to when hold_after ends, e.g. "Time"
	RequireArm bool     `yaml:"require_arm"` // hidden until armed (admin UI / arm_topic); disarms itself after the next arrival

	// Extra fields merged verbatim into the AWTRIX payload (duration, rainbow, pushIcon, ...).
	Extra map[string]any `yaml:"extra"`
}

// Load reads the file, expands ${ENV} references and validates the result.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse expands ${ENV} references in raw YAML and validates the result.
func Parse(raw []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(raw))), &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &c, c.validate()
}

func (c *Config) applyDefaults() {
	if c.MQTT.ClientID == "" {
		c.MQTT.ClientID = "awtrix-bridge"
	}
	if c.Awtrix.AppTopic == "" {
		c.Awtrix.AppTopic = "awtrix/cmd/apps/pushed/{name}"
	}
	if c.Awtrix.SettingsTopic == "" {
		c.Awtrix.SettingsTopic = "awtrix/cmd/settings"
	}
	if c.Awtrix.SoundTopic == "" {
		c.Awtrix.SoundTopic = "awtrix/cmd/audio/play"
	}
	if c.Awtrix.ArmTopic == "" {
		c.Awtrix.ArmTopic = "awtrix-bridge/{name}/armed"
	}
	if c.Awtrix.ResyncInterval == 0 {
		c.Awtrix.ResyncInterval = Duration(5 * time.Minute)
	}
	for i := range c.Apps {
		if c.Apps[i].Scale == 0 {
			c.Apps[i].Scale = 1
		}
	}
}

func (c *Config) validate() error {
	if c.HomeAssistant.URL == "" || c.HomeAssistant.Token == "" {
		return fmt.Errorf("homeassistant.url and homeassistant.token are required")
	}
	if c.MQTT.Broker == "" {
		return fmt.Errorf("mqtt.broker is required")
	}
	seen := map[string]bool{}
	for _, a := range c.Apps {
		if a.Name == "" {
			return fmt.Errorf("app without name")
		}
		if seen[a.Name] {
			return fmt.Errorf("duplicate app name %q", a.Name)
		}
		seen[a.Name] = true
		switch a.Type {
		case "value", "countdown":
			if a.Entity == "" {
				return fmt.Errorf("app %q: entity is required", a.Name)
			}
		case "surplus":
			if a.Production == "" || a.Consumption == "" {
				return fmt.Errorf("app %q: production and consumption are required", a.Name)
			}
		default:
			return fmt.Errorf("app %q: unknown type %q", a.Name, a.Type)
		}
	}
	return nil
}
