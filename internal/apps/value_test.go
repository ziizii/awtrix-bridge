package apps

import (
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

func TestValueThresholds(t *testing.T) {
	a := &valueApp{topic: "t", cfg: config.App{
		Entity: "sensor.radon", Icon: "2644", Format: "Rn %.0f Bq", Scale: 1,
		Colors: []config.Threshold{{Below: ptr(100), Color: "green"}, {Below: ptr(300), Color: "orange"}, {Color: "red"}},
	}}
	for state, want := range map[string]string{"34": "green", "150": "orange", "999": "red"} {
		m := decode(t, a.Render(fake{"sensor.radon": state}, time.Now())[0].Payload)
		if m["textColor"] != want {
			t.Errorf("%s: color %v, want %s", state, m["textColor"], want)
		}
	}
	m := decode(t, a.Render(fake{"sensor.radon": "34"}, time.Now())[0].Payload)
	if m["text"] != "Rn 34 Bq" || m["icon"] != "2644" {
		t.Errorf("payload %v", m)
	}
	if got := a.Render(fake{"sensor.radon": "unavailable"}, time.Now())[0].Payload; got != nil {
		t.Errorf("unavailable should remove app, got %s", got)
	}
}

func TestFormatPower(t *testing.T) {
	cases := map[float64]string{2900: "+2.9kW", -2900: "-2.9kW", 850: "+850W", -850: "-850W", 0: "0W"}
	for in, want := range cases {
		if got := FormatPower(in, true); got != want {
			t.Errorf("FormatPower(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSurplusNightIcon(t *testing.T) {
	a := &surplusApp{topic: "t", cfg: config.App{Production: "pv", Consumption: "load", Icon: "sun", NightIcon: "moon"}}
	m := decode(t, a.Render(fake{"pv": "0", "load": "2900"}, time.Now())[0].Payload)
	if m["icon"] != "moon" || m["text"] != "-2.9kW" || m["textColor"] != "#FF4400" {
		t.Errorf("payload %v", m)
	}
}
