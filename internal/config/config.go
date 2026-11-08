// Package config loads, validates and edits the YAML configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HomeAssistant HomeAssistant `yaml:"homeassistant"`
	MQTT          MQTT          `yaml:"mqtt"`
	Awtrix        Awtrix        `yaml:"awtrix"`
	Brightness    *Brightness   `yaml:"brightness"`
	Chime         *Chime        `yaml:"chime"`
	Timer         *Timer        `yaml:"timer"` // always set after Parse
	Apps          []App         `yaml:"apps"`
}

type HomeAssistant struct {
	URL   string `yaml:"url"` // HA base URL, e.g. http://homeassistant.local:8123
	Token string `yaml:"token"`
}

type MQTT struct {
	Broker   string `yaml:"broker"` // tcp://host:1883
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	ClientID string `yaml:"client_id"`
}

// Awtrix holds the clock's MQTT topics. "{name}" in a topic is replaced
// with the app name.
type Awtrix struct {
	AppTopic      string `yaml:"app_topic"`
	SettingsTopic string `yaml:"settings_topic"`
	SoundTopic    string `yaml:"sound_topic"`
	SwitchTopic   string `yaml:"switch_topic"`
	ActiveTopic   string `yaml:"active_topic"` // where the clock reports the app on screen
	// ClockSettingsTopic is where the clock reports its own settings (time
	// format, colors); the admin UI draws its preview with them.
	ClockSettingsTopic string `yaml:"clock_settings_topic"`
	ArmTopic           string `yaml:"arm_topic"` // retained on/off arm state of require_arm apps
	// TimerTopic holds the timer's retained state (JSON with start, end
	// and the app to return to); anything can start a timer through it.
	TimerTopic string `yaml:"timer_topic"`
	// ResyncInterval re-publishes every app periodically so the clock recovers after a reboot.
	ResyncInterval Duration `yaml:"resync_interval"`
}

// Brightness switches the clock to Quiet brightness while QuietEntity is on.
type Brightness struct {
	QuietEntity string `yaml:"quiet_entity"`
	Quiet       int    `yaml:"quiet"`
	Normal      int    `yaml:"normal"`
}

// Chime plays a melody at every full hour.
type Chime struct {
	Melody      string `yaml:"melody"`       // a Melodies name, "off", or empty to use RTTTL
	RTTTL       string `yaml:"rtttl"`        // custom tune when Melody is empty
	QuietEntity string `yaml:"quiet_entity"` // on => stay silent
}

// Tune returns the RTTTL to play, or "" when the chime is off.
func (c Chime) Tune() string { return tune(c.Melody, c.RTTTL) }

// TimerApp is the name of the timer's pushed app on the clock.
const TimerApp = "timer"

// Timer is the timer started from the admin UI: a duration or until a
// time of day. While it runs the clock shows it; when it ends it plays a
// melody, counts up for HoldAfter, then switches back.
type Timer struct {
	Icon      string `yaml:"icon"`
	Color     string `yaml:"color"`      // text while running
	DoneColor string `yaml:"done_color"` // text after it ends
	// The progress bar on the bottom row: the elapsed part and the rest.
	ProgressColor string   `yaml:"progress_color"`
	TrackColor    string   `yaml:"track_color"`
	Melody        string   `yaml:"melody"`      // a Melodies name or "off"; default beeps
	RTTTL         string   `yaml:"rtttl"`       // custom tune when Melody is empty
	HoldAfter     Duration `yaml:"hold_after"`  // count up this long after it ends (default 1m)
	SwitchBack    string   `yaml:"switch_back"` // app to show afterwards; default: the one on screen when it started
}

// Tune returns the RTTTL to play when the timer ends, or "" for none.
func (t Timer) Tune() string { return tune(t.Melody, t.RTTTL) }

// Threshold selects a color: the first entry whose Below is greater than the
// value wins; an entry without Below is the fallback.
type Threshold struct {
	Below *float64 `yaml:"below"`
	Color string   `yaml:"color"`
}

// App types.
const (
	TypeValue     = "value"
	TypeSurplus   = "surplus"
	TypeCountdown = "countdown"
)

// App is one entry of the apps list. Fields are grouped by the app types
// that use them.
type App struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // value | surplus | countdown

	// value, countdown
	Entity string      `yaml:"entity"`
	Icon   string      `yaml:"icon"`
	Color  string      `yaml:"color"`
	Colors []Threshold `yaml:"colors"` // countdown: thresholds in minutes remaining

	// value
	Format string  `yaml:"format"` // fmt verb applied to the numeric state, e.g. "%.0fW"
	Scale  float64 `yaml:"scale"`  // multiply the state before formatting (default 1)

	// surplus
	Production    string `yaml:"production"`
	Consumption   string `yaml:"consumption"`
	NightIcon     string `yaml:"night_icon"` // icon when production is 0
	PositiveColor string `yaml:"positive_color"`
	NegativeColor string `yaml:"negative_color"`

	// countdown
	ShowWithin  Duration `yaml:"show_within"`  // only show when arrival is within this window (0 = always)
	HoldAfter   Duration `yaml:"hold_after"`   // count up ("+0:42") this long after arrival
	AlertAt     Duration `yaml:"alert_at"`     // play the alert once when remaining drops below this
	AlertMelody string   `yaml:"alert_melody"` // built-in melody name or "off"; overrides alert_rtttl
	AlertRTTTL  string   `yaml:"alert_rtttl"`  // custom alert tune
	SwitchAt    Duration `yaml:"switch_at"`    // switch the clock to this app once when remaining drops below this (0 = never)
	SwitchBack  string   `yaml:"switch_back"`  // app to switch back to when hold_after ends, e.g. "Time"
	RequireArm  bool     `yaml:"require_arm"`  // hidden until armed; disarms itself after the next arrival

	// Extra fields merged verbatim into the AWTRIX payload (duration, rainbow, pushIcon, ...).
	Extra map[string]any `yaml:"extra"`
}

// AlertTune returns the countdown alert's RTTTL, or "" for no alert.
func (a App) AlertTune() string { return tune(a.AlertMelody, a.AlertRTTTL) }

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

// Parse expands ${ENV} references in raw YAML, applies defaults and
// validates the result.
func Parse(raw []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(raw))), &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &c, c.validate()
}

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
