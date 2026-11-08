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
