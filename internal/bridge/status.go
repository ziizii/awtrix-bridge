package bridge

import (
	"encoding/json"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/engine"
)

// Status is everything the admin UI shows; it is polled every second.
type Status struct {
	Started    time.Time          `json:"started"`
	Loaded     time.Time          `json:"loaded"`          // last config (re)load
	MQTT       bool               `json:"mqtt"`            // connected to the broker
	Active     string             `json:"active"`          // app on screen, as the clock reports it
	Clock      json.RawMessage    `json:"clock,omitempty"` // the clock's own settings, as it reports them
	Apps       []engine.Published `json:"apps"`            // what the clock currently has
	Recent     []engine.Published `json:"recent"`          // latest switches, sounds, arm changes
	Countdowns []CountdownStatus  `json:"countdowns"`
	Chime      ChimeStatus        `json:"chime"`
	Melodies   []config.Melody    `json:"melodies"` // built-in tunes for chime and alerts
}

// CountdownStatus is a countdown app's arming and alert setting.
type CountdownStatus struct {
	App          string `json:"app"`
	RequireArm   bool   `json:"require_arm"`
	Armed        bool   `json:"armed"`
	AlertMinutes int    `json:"alert_minutes"` // 0 = no alert
	AlertMelody  string `json:"alert_melody"`  // melody name, "off", or "custom"
}

// ChimeStatus is the hourly chime choice and what silences it.
type ChimeStatus struct {
	Melody string        `json:"melody"` // melody name, "off", or "custom"
	Quiet  *EntityStatus `json:"quiet,omitempty"`
}

// EntityStatus is an HA entity as the bridge last saw it.
type EntityStatus struct {
	Entity string `json:"entity"`
	Name   string `json:"name"`
	State  string `json:"state"`
	On     bool   `json:"on"`
	From   string `json:"from,omitempty"` // time-of-day sensors: "after" attribute
	To     string `json:"to,omitempty"`   // time-of-day sensors: "before" attribute
}

func (s *Supervisor) Status() Status {
	s.mu.Lock()
	inst := s.cur
	st := Status{Started: s.started, Loaded: s.loaded, Melodies: config.Melodies, Countdowns: []CountdownStatus{}}
	s.mu.Unlock()
	if inst == nil {
		return st
	}
	st.MQTT = inst.mqtt.IsConnectionOpen()
	st.Active = inst.activeApp()
	st.Clock = inst.clockSettings()
	st.Apps = inst.eng.Shown()
	st.Recent = inst.eng.Recent()
	st.Chime = inst.chimeStatus()
	for _, a := range inst.cfg.Apps {
		if a.Type == config.TypeCountdown {
			st.Countdowns = append(st.Countdowns, CountdownStatus{
				App:          a.Name,
				RequireArm:   a.RequireArm,
				Armed:        inst.eng.Armed(a.Name),
				AlertMinutes: int(a.AlertAt.D().Minutes()),
				AlertMelody:  melodyChoice(a.AlertMelody, a.AlertRTTTL),
			})
		}
	}
	return st
}

func (i *instance) chimeStatus() ChimeStatus {
	ch := i.cfg.Chime
	if ch == nil {
		return ChimeStatus{Melody: config.MelodyOff}
	}
	st := ChimeStatus{Melody: melodyChoice(ch.Melody, ch.RTTTL)}
	if ch.QuietEntity != "" {
		st.Quiet = i.entityStatus(ch.QuietEntity)
	}
	return st
}

func (i *instance) entityStatus(id string) *EntityStatus {
	es := &EntityStatus{Entity: id, State: "unknown"}
	if hs, ok := i.eng.State(id); ok {
		es.State, es.On = hs.State, hs.State == "on"
		es.Name, _ = hs.Attributes["friendly_name"].(string)
		es.From, _ = hs.Attributes["after"].(string)
		es.To, _ = hs.Attributes["before"].(string)
	}
	return es
}

// melodyChoice names a melody setting for the UI: the melody, "custom"
// for a custom RTTTL, or "off".
func melodyChoice(melody, custom string) string {
	switch {
	case melody != "":
		return melody
	case custom != "":
		return "custom"
	}
	return config.MelodyOff
}
