package config

import (
	"testing"
	"time"
)

func TestParseCountdown(t *testing.T) {
	t.Setenv("HA_TOKEN", "secret")
	c, err := Parse([]byte(`
homeassistant: {url: "http://ha:8123", token: "${HA_TOKEN}"}
mqtt: {broker: "tcp://b:1883"}
apps:
  - {name: bus333, type: countdown, entity: sensor.bus, require_arm: true, switch_at: 10m, switch_back: Time, hold_after: 2m}
`))
	if err != nil {
		t.Fatal(err)
	}
	a := c.Apps[0]
	if c.HomeAssistant.Token != "secret" || !a.RequireArm || a.SwitchAt.D() != 10*time.Minute || a.SwitchBack != "Time" {
		t.Errorf("parsed %+v", a)
	}
	if c.Awtrix.ArmTopic != "awtrix-bridge/{name}/armed" {
		t.Errorf("arm topic default %q", c.Awtrix.ArmTopic)
	}
	if _, err := Parse([]byte("homeassistant: {url: x}")); err == nil {
		t.Error("missing token accepted")
	}
}

func TestChimeTune(t *testing.T) {
	if got := (Chime{Melody: "ding"}).Tune(); got != "ding:d=16,o=6,b=120:e" {
		t.Errorf("ding: %q", got)
	}
	if got := (Chime{Melody: "off", RTTTL: "x"}).Tune(); got != "" {
		t.Errorf("off: %q", got)
	}
	if got := (Chime{RTTTL: "custom:d=4:c"}).Tune(); got != "custom:d=4:c" {
		t.Errorf("custom: %q", got)
	}
	if _, err := Parse([]byte(base + "chime: {melody: nope}\n")); err == nil {
		t.Error("unknown melody accepted")
	}
}

func TestValidateAlertMelody(t *testing.T) {
	_, err := Parse([]byte(base + "apps:\n  - {name: bus, type: countdown, entity: sensor.bus, alert_melody: nope}\n"))
	if err == nil || err.Error() != `app "bus": alert_melody: unknown melody "nope"` {
		t.Errorf("got %v", err)
	}
}
