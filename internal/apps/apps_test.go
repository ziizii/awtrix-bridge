package apps

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

type fake map[string]string

func (f fake) Get(id string) (ha.State, bool) {
	v, ok := f[id]
	return ha.State{EntityID: id, State: v}, ok
}

func ptr(f float64) *float64 { return &f }

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("bad json %q: %v", b, err)
	}
	return m
}

var topics = Topics{App: "awtrix/cmd/apps/pushed/{name}", Settings: "s", Sound: "snd", Switch: "awtrix/cmd/apps/switch"}

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

func TestCountdown(t *testing.T) {
	now := time.Date(2026, 11, 7, 16, 0, 0, 0, time.UTC)
	arrival := now.Add(4*time.Minute + 5*time.Second).Format(time.RFC3339)
	a := &countdownApp{topics: topics, cfg: config.App{
		Name: "bus333", Entity: "sensor.bus", Icon: "11999",
		ShowWithin: config.Duration(30 * time.Minute), HoldAfter: config.Duration(30 * time.Second),
		AlertAt: config.Duration(5 * time.Minute), AlertRTTTL: "beep", SwitchTo: true,
		Colors: []config.Threshold{{Below: ptr(2), Color: "red"}, {Below: ptr(6), Color: "orange"}, {Color: "green"}},
	}}
	st := fake{"sensor.bus": arrival}

	msgs := a.Render(st, now)
	if len(msgs) != 3 {
		t.Fatalf("first render: want app+switch+sound, got %d msgs", len(msgs))
	}
	m := decode(t, msgs[0].Payload)
	if m["text"] != "4:05" || m["textColor"] != "orange" {
		t.Errorf("payload %v", m)
	}
	if msgs[1].Topic != "awtrix/cmd/apps/switch" || msgs[2].Topic != "snd" {
		t.Errorf("unexpected topics %q %q", msgs[1].Topic, msgs[2].Topic)
	}

	// Next second: only the app update, no repeated switch/sound.
	if msgs := a.Render(st, now.Add(time.Second)); len(msgs) != 1 || decode(t, msgs[0].Payload)["text"] != "4:04" {
		t.Errorf("second render: %v", msgs)
	}
	// At arrival.
	if m := decode(t, a.Render(st, now.Add(4*time.Minute+5*time.Second))[0].Payload); m["text"] != "Now!" || m["textColor"] != "red" {
		t.Errorf("arrival payload %v", m)
	}
	// After hold window: removed.
	if p := a.Render(st, now.Add(5*time.Minute)); p[0].Payload != nil {
		t.Errorf("expected removal, got %s", p[0].Payload)
	}
	// Too far in the future: hidden.
	if p := a.Render(fake{"sensor.bus": now.Add(time.Hour).Format(time.RFC3339)}, now); p[0].Payload != nil {
		t.Errorf("expected hidden, got %s", p[0].Payload)
	}
}

func TestCountdownAlertOncePerBus(t *testing.T) {
	now := time.Date(2026, 11, 7, 16, 17, 0, 0, time.UTC)
	a := &countdownApp{topics: topics, cfg: config.App{
		Name: "bus333", Entity: "sensor.bus", ShowWithin: config.Duration(30 * time.Minute),
		AlertAt: config.Duration(5 * time.Minute), AlertRTTTL: "beep",
	}}
	sounds := func(msgs []Message) (n int) {
		for _, m := range msgs {
			if m.Topic == "snd" {
				n++
			}
		}
		return
	}
	// Live estimates for one bus, as sensor.next_333_arrival reports them
	// minute by minute: the target moves by seconds each update.
	estimates := []string{"16:21:21", "16:21:31", "16:22:14", "16:21:48", "16:22:04"}
	total := 0
	for i, e := range estimates {
		target, _ := time.Parse(time.RFC3339, "2026-11-07T"+e+"Z")
		total += sounds(a.Render(fake{"sensor.bus": target.Format(time.RFC3339)}, now.Add(time.Duration(i)*time.Minute)))
	}
	if total != 1 {
		t.Errorf("want 1 alert for one bus, got %d", total)
	}
	// The next bus two hours later alerts again.
	next := time.Date(2026, 11, 7, 18, 18, 0, 0, time.UTC)
	if n := sounds(a.Render(fake{"sensor.bus": next.Format(time.RFC3339)}, next.Add(-4*time.Minute))); n != 1 {
		t.Errorf("next bus: want 1 alert, got %d", n)
	}
}

func TestSwitchTopic(t *testing.T) {
	if got := switchTopic("awtrix/cmd/apps/pushed/{name}"); got != "awtrix/cmd/apps/switch" {
		t.Errorf("got %q", got)
	}
}
