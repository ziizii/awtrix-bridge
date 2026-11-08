// Package awtrix knows the clock's MQTT protocol: which topic does what and
// how payloads look. Nothing else in the bridge builds clock JSON.
package awtrix

import (
	"encoding/json"
	"strings"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

// BuiltinApps are apps the clock firmware provides itself; they can be
// switched to but are not pushed by the bridge.
var BuiltinApps = []string{"Time", "Date"}

// Topics are the MQTT topics of one clock (and of the bridge's own state).
type Topics struct {
	Settings string
	Sound    string
	Switch   string
	Active   string // the clock reports the app on screen here
	app      string // contains {name}
	arm      string // contains {name}
}

func NewTopics(c config.Awtrix) Topics {
	return Topics{
		Settings: c.SettingsTopic,
		Sound:    c.SoundTopic,
		Switch:   c.SwitchTopic,
		Active:   c.ActiveTopic,
		app:      c.AppTopic,
		arm:      c.ArmTopic,
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
