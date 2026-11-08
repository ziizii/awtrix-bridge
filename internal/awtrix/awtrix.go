// Package awtrix knows the clock's MQTT protocol: which topic does what and
// how payloads look. Nothing else in the bridge builds clock JSON.
package awtrix

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

// BuiltinApps are apps the clock firmware provides itself; they can be
// switched to but are not pushed by the bridge.
var BuiltinApps = []string{"Time", "Date"}

// Topics are the MQTT topics of one clock (and of the bridge's own state).
type Topics struct {
	Settings      string
	Sound         string
	Switch        string
	Active        string // the clock reports the app on screen here
	ClockSettings string // the clock reports its own settings here (JSON)
	Timer         string // the bridge's retained timer state (JSON)
	Device        string // the clock reports itself here (JSON with its IP)
	app           string // contains {name}
	arm           string // contains {name}
}

func NewTopics(c config.Awtrix) Topics {
	return Topics{
		Settings:      c.SettingsTopic,
		Sound:         c.SoundTopic,
		Switch:        c.SwitchTopic,
		Active:        c.ActiveTopic,
		ClockSettings: c.ClockSettingsTopic,
		Timer:         c.TimerTopic,
		Device:        c.DeviceTopic,
		app:           c.AppTopic,
		arm:           c.ArmTopic,
	}
}

// App is the topic of a pushed (custom) app; an empty payload removes it.
func (t Topics) App(name string) string { return strings.ReplaceAll(t.app, "{name}", name) }

// Arm holds the retained "on"/"off" arm state of an app.
func (t Topics) Arm(name string) string { return strings.ReplaceAll(t.arm, "{name}", name) }

// AppPayload is the content of a pushed app. Extra fields are merged in
// verbatim (duration, rainbow, pushIcon, ...).
func AppPayload(icon, text, color string, extra map[string]any) []byte {
	m := map[string]any{"text": text}
	if icon != "" {
		m["icon"] = icon
	}
	if color != "" {
		m["textColor"] = color
	}
	for k, v := range extra {
		m[k] = v
	}
	return marshal(m)
}

// SwitchPayload shows an app now.
func SwitchPayload(app string) []byte { return marshal(map[string]any{"name": app, "fast": true}) }

// SoundPayload plays an RTTTL tune on the buzzer.
func SoundPayload(rtttl string) []byte { return marshal(map[string]string{"rtttl": rtttl}) }

// BrightnessPayload sets a fixed brightness (0-255).
func BrightnessPayload(b int) []byte {
	return marshal(map[string]any{"brightness": b, "autoBrightness": false})
}

// ArmPayload is the retained arm state.
func ArmPayload(on bool) []byte {
	if on {
		return []byte("on")
	}
	return []byte("off")
}

// IsArmed parses an arm state payload.
func IsArmed(payload []byte) bool { return string(payload) == "on" }

// marshal encodes maps built here or decoded from YAML (yaml.v3 yields
// map[string]any), which always encode.
func marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// Timer is the timer's state, kept retained on the timer topic so a
// running timer survives restarts. The zero Timer means none.
type Timer struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Back  string    `json:"back,omitempty"` // app to switch back to afterwards
}

func (t Timer) Running() bool { return !t.End.IsZero() }

// TimerPayload encodes a timer; an empty payload clears the retained state.
func TimerPayload(t Timer) []byte {
	if !t.Running() {
		return []byte{}
	}
	return marshal(t)
}

// ParseTimer decodes a timer state; anything unreadable means no timer.
func ParseTimer(payload []byte) Timer {
	var t Timer
	if json.Unmarshal(payload, &t) != nil || t.End.IsZero() {
		return Timer{}
	}
	if t.Start.IsZero() || t.Start.After(t.End) {
		t.Start = t.End
	}
	return t
}

// ResultTopics are where the clock answers commands: each command topic
// plus "/result", with "+" for the app name of pushed apps.
func (t Topics) ResultTopics() []string {
	out := []string{}
	for _, c := range []string{t.Settings, t.Sound, t.Switch, t.App("+")} {
		if c != "" {
			out = append(out, c+"/result")
		}
	}
	return out
}

// CommandOf returns the command topic a result topic answers.
func CommandOf(resultTopic string) string { return strings.TrimSuffix(resultTopic, "/result") }

// Result is the clock's answer to a command.
type Result struct {
	OK    bool `json:"ok"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"error"`
}

// Reason describes why the clock rejected a command.
func (r Result) Reason() string {
	if r.Error == nil {
		return "rejected"
	}
	msg := r.Error.Message
	if msg == "" {
		msg = r.Error.Code
	}
	if r.Error.Field != "" && !strings.Contains(msg, r.Error.Field) {
		msg += " (" + r.Error.Field + ")"
	}
	return msg
}

// ParseResult decodes a result; false if it is not one.
func ParseResult(payload []byte) (Result, bool) {
	var r Result
	if json.Unmarshal(payload, &r) != nil {
		return Result{}, false
	}
	return r, true
}
