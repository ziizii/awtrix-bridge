package config

import (
	"strings"
	"testing"
	"time"
)

const base = `homeassistant: {url: "http://ha", token: t}
mqtt: {broker: "tcp://b:1883"}
`

func TestSetChime(t *testing.T) {
	ding := []Field{{"melody", "ding"}}
	cases := map[string]struct{ in, want string }{
		"replace custom rtttl": {
			in:   base + "chime:                # hourly\n  rtttl: \"hour:d=4:c\"\n  quiet_entity: q   # silent\n\napps: []\n",
			want: "chime:                # hourly\n  melody: ding\n  # rtttl: \"hour:d=4:c\"\n  quiet_entity: q   # silent\n",
		},
		"change melody, keep comment": {
			in:   base + "chime:\n  melody: tick    # pick in the UI\n  quiet_entity: q\napps: []\n",
			want: "chime:\n  melody: ding    # pick in the UI\n  quiet_entity: q\napps: []\n",
		},
		"add block": {
			in:   base + "apps: []\n",
			want: "chime:\n  melody: ding\n\napps: []\n",
		},
	}
	for name, c := range cases {
		out := string(SetChime([]byte(c.in), ding, "rtttl"))
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: got\n%s", name, out)
			continue
		}
		cfg, err := Parse([]byte(out))
		if err != nil || cfg.Chime == nil || cfg.Chime.Melody != "ding" || cfg.Chime.Tune() == "" {
			t.Errorf("%s: parsed %+v, %v", name, cfg, err)
		}
	}
}

func TestSetApp(t *testing.T) {
	in := base + `apps:
  - name: radon
    type: value
    entity: sensor.radon_level
  - name: bus333                  # the bus
    type: countdown
    entity: sensor.next_333_arrival
    alert_at: 5m                  # beep once when 5 minutes remain
    alert_rtttl: "two:d=4,o=5,b=200:c,e"
    colors:                       # thresholds in minutes remaining
      - { below: 2, color: "#FF2200" }
`
	out, err := SetApp([]byte(in), "bus333", []Field{{"alert_at", "7m"}, {"alert_melody", "beeps"}}, "alert_rtttl")
	if err != nil {
		t.Fatal(err)
	}
	want := `    alert_at: 7m                  # beep once when 5 minutes remain
    alert_melody: beeps
    # alert_rtttl: "two:d=4,o=5,b=200:c,e"
    colors:                       # thresholds in minutes remaining
      - { below: 2, color: "#FF2200" }
`
	if !strings.Contains(string(out), want) {
		t.Errorf("got\n%s", out)
	}
	if strings.Count(string(out), "alert_at") != 1 {
		t.Errorf("radon entry must stay untouched:\n%s", out)
	}
	cfg, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	bus := cfg.Apps[1]
	if bus.AlertAt.D().Minutes() != 7 || bus.AlertTune() != "beeps:d=16,o=6,b=180:c,p,c,p,c" || len(bus.Colors) != 1 {
		t.Errorf("parsed %+v", bus)
	}
	if _, err := SetApp([]byte(in), "nope", nil); err == nil {
		t.Error("unknown app accepted")
	}
}

func TestSetSectionTimer(t *testing.T) {
	in := base + "chime:\n  melody: tick\n\napps: []\n"
	out := string(SetSection([]byte(in), "timer", []Field{{"melody", "ding"}}, "rtttl"))
	if !strings.Contains(out, "chime:\n  melody: tick\n\ntimer:\n  melody: ding\n\napps: []\n") {
		t.Errorf("got:\n%s", out)
	}
	c, err := Parse([]byte(out))
	if err != nil || c.Timer.Melody != "ding" {
		t.Errorf("parse: %v %+v", err, c.Timer)
	}
}

func TestTimerDefaults(t *testing.T) {
	c, err := Parse([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	if c.Timer == nil || c.Timer.Melody != "beeps" || c.Timer.HoldAfter.D() != time.Minute || c.Awtrix.TimerTopic != "awtrix-bridge/timer" {
		t.Errorf("defaults: %+v %q", c.Timer, c.Awtrix.TimerTopic)
	}
	if _, err := Parse([]byte(base + "apps:\n  - {name: timer, type: value, entity: x}\n")); err == nil {
		t.Errorf("app named timer accepted")
	}
}

func TestSetThresholdColor(t *testing.T) {
	in := base + `apps:
  - name: radon
    type: value
    entity: e
    color: "#FFFFFF"              # fallback
    colors:                       # first match wins
      - { below: 100, color: "#00FF00" }
      - { below: 300, color: '#FFAA00' }   # orange
      - { color: "#FF2200" }

  - name: bus
    type: countdown
    entity: e
    colors:
    - below: 2
      color: "#FF2200"
    - color: "#00FF00"
`
	out, err := SetThresholdColor([]byte(in), "radon", 1, "#123456")
	if err != nil || !strings.Contains(string(out), `      - { below: 300, color: "#123456" }   # orange`) ||
		!strings.Contains(string(out), `color: "#FFFFFF"              # fallback`) {
		t.Errorf("flow entry: %v\n%s", err, out)
	}
	out, err = SetThresholdColor([]byte(in), "bus", 1, "#ABCDEF")
	if err != nil || !strings.Contains(string(out), "    - color: \"#ABCDEF\"\n") || !strings.Contains(string(out), `      color: "#FF2200"`) {
		t.Errorf("block entry: %v\n%s", err, out)
	}
	out, _ = SetThresholdColor([]byte(in), "bus", 0, "#000001")
	if !strings.Contains(string(out), "    - below: 2\n      color: \"#000001\"") {
		t.Errorf("block entry 0:\n%s", out)
	}
	if _, err := SetThresholdColor([]byte(in), "radon", 3, "#000000"); err == nil {
		t.Errorf("entry 3 accepted")
	}
	if _, err := Parse(out); err != nil {
		t.Errorf("edited file does not parse: %v", err)
	}
}

func TestCheckColor(t *testing.T) {
	for c, ok := range map[string]bool{"#FFaa00": true, "#FFF": false, "FFAA00": false, "#GGGGGG": false, "": false} {
		if (CheckColor(c) == nil) != ok {
			t.Errorf("%q: want ok=%v", c, ok)
		}
	}
}
