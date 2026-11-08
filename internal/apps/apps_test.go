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
		AlertAt: config.Duration(5 * time.Minute), AlertRTTTL: "beep", SwitchAt: config.Duration(10 * time.Minute),
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
	// After arrival it counts up.
	if m := decode(t, a.Render(st, now.Add(4*time.Minute+17*time.Second))[0].Payload); m["text"] != "+0:12" || m["textColor"] != "red" {
		t.Errorf("count-up payload %v", m)
	}
	// After hold window: removed.
	if p := a.Render(st, now.Add(5*time.Minute)); p[len(p)-1].Payload != nil {
		t.Errorf("expected removal, got %s", p[len(p)-1].Payload)
	}
	// Too far in the future: hidden.
	if p := a.Render(fake{"sensor.bus": now.Add(time.Hour).Format(time.RFC3339)}, now); p[0].Payload != nil {
		t.Errorf("expected hidden, got %s", p[0].Payload)
	}
}

func TestCountdownSwitchAtZeroNever(t *testing.T) {
	arrival := time.Date(2026, 11, 7, 19, 18, 0, 0, time.UTC)
	a := &countdownApp{topics: topics, cfg: config.App{Name: "bus333", Entity: "sensor.bus", SwitchBack: "Time"}}
	st := fake{"sensor.bus": arrival.Format(time.RFC3339)}
	for d := -15 * time.Minute; d < 5*time.Minute; d += 30 * time.Second {
		for _, m := range a.Render(st, arrival.Add(d)) {
			if m.Topic == topics.Switch {
				t.Fatalf("switch_at 0 must never switch, got %s at %v", m.Payload, d)
			}
		}
	}
}

// The feed jumps to the next bus ~20s after one arrives (seen in HA
// history). The app must keep counting up the bus that arrived, then
// switch the clock back once.
func TestCountdownCountUpAndSwitchBack(t *testing.T) {
	arrival := time.Date(2026, 11, 8, 6, 19, 15, 0, time.UTC)
	next := time.Date(2026, 11, 8, 8, 18, 0, 0, time.UTC)
	a := &countdownApp{topics: topics, cfg: config.App{
		Name: "bus333", Entity: "sensor.bus", HoldAfter: config.Duration(2 * time.Minute),
		SwitchAt: config.Duration(10 * time.Minute), SwitchBack: "Time",
	}}
	var switches []string
	render := func(target time.Time, now time.Time) map[string]any {
		var app map[string]any
		for _, m := range a.Render(fake{"sensor.bus": target.Format(time.RFC3339)}, now) {
			switch m.Topic {
			case topics.Switch:
				switches = append(switches, decode(t, m.Payload)["name"].(string))
			case topics.app("bus333"):
				if m.Payload != nil {
					app = decode(t, m.Payload)
				}
			}
		}
		return app
	}

	render(arrival, arrival.Add(-9*time.Minute)) // switches to bus333
	if m := render(arrival, arrival.Add(5*time.Second)); m["text"] != "+0:05" {
		t.Errorf("at +5s: %v", m)
	}
	// Feed already reports the next bus, but we keep counting up.
	if m := render(next, arrival.Add(21*time.Second)); m["text"] != "+0:21" {
		t.Errorf("after feed jump: %v", m)
	}
	if m := render(next, arrival.Add(2*time.Minute)); m["text"] != "+2:00" {
		t.Errorf("end of hold: %v", m)
	}
	if len(switches) != 1 {
		t.Fatalf("before hold ends: switches %v", switches)
	}
	// Hold over: next bus shown, clock switched back to Time once.
	if m := render(next, arrival.Add(2*time.Minute+time.Second)); m["text"] != "116:44" {
		t.Errorf("after hold: %v", m)
	}
	render(next, arrival.Add(2*time.Minute+2*time.Second))
	if want := []string{"bus333", "Time"}; len(switches) != 2 || switches[0] != want[0] || switches[1] != want[1] {
		t.Errorf("switches %v, want %v", switches, want)
	}
}

func TestCarryKeepsSwitchState(t *testing.T) {
	cfg := &config.Config{
		Awtrix: config.Awtrix{AppTopic: "awtrix/cmd/apps/pushed/{name}"},
		Apps:   []config.App{{Name: "bus333", Type: "countdown", Entity: "sensor.bus", SwitchAt: config.Duration(10 * time.Minute)}},
	}
	old, _ := Build(cfg)
	arrival := time.Date(2026, 11, 7, 19, 18, 0, 0, time.UTC)
	st := fake{"sensor.bus": arrival.Format(time.RFC3339)}
	old[0].Render(st, arrival.Add(-9*time.Minute))

	fresh, _ := Build(cfg)
	Carry(old, fresh)
	for _, m := range fresh[0].Render(st, arrival.Add(-8*time.Minute)) {
		if m.Topic == "awtrix/cmd/apps/switch" {
			t.Errorf("reloaded app switched again")
		}
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

func TestCountdownSwitchAt(t *testing.T) {
	arrival := time.Date(2026, 11, 7, 19, 18, 0, 0, time.UTC)
	a := &countdownApp{topics: topics, cfg: config.App{
		Name: "bus333", Entity: "sensor.bus",
		SwitchAt: config.Duration(10 * time.Minute),
	}}
	switches := func(msgs []Message) (n int) {
		for _, m := range msgs {
			if m.Topic == topics.Switch {
				n++
			}
		}
		return
	}
	st := fake{"sensor.bus": arrival.Format(time.RFC3339)}
	// Shown long before arrival (show_within unset), but no switch yet.
	if n := switches(a.Render(st, arrival.Add(-88*time.Minute))); n != 0 {
		t.Errorf("88 min out: want no switch, got %d", n)
	}
	// Crossing 10 minutes switches once, even as the estimate drifts.
	total := 0
	for i, drift := range []time.Duration{0, 12 * time.Second, -20 * time.Second, 31 * time.Second} {
		st := fake{"sensor.bus": arrival.Add(drift).Format(time.RFC3339)}
		total += switches(a.Render(st, arrival.Add(-10*time.Minute+time.Duration(i)*time.Minute)))
	}
	if total != 1 {
		t.Errorf("want 1 switch for one bus, got %d", total)
	}
	// The next bus switches again at its own 10-minute mark.
	next := arrival.Add(2 * time.Hour)
	if n := switches(a.Render(fake{"sensor.bus": next.Format(time.RFC3339)}, next.Add(-9*time.Minute))); n != 1 {
		t.Errorf("next bus: want 1 switch, got %d", n)
	}
}

func TestSwitchTopic(t *testing.T) {
	if got := SwitchTopic("awtrix/cmd/apps/pushed/{name}"); got != "awtrix/cmd/apps/switch" {
		t.Errorf("got %q", got)
	}
}

func TestCountdownRequireArm(t *testing.T) {
	arrival := time.Date(2026, 11, 8, 6, 19, 15, 0, time.UTC)
	next := arrival.Add(2 * time.Hour)
	tp := topics
	tp.Arm = "awtrix-bridge/{name}/armed"
	a := &countdownApp{topics: tp, cfg: config.App{
		Name: "bus333", Entity: "sensor.bus", RequireArm: true, HoldAfter: config.Duration(2 * time.Minute),
		SwitchAt: config.Duration(10 * time.Minute), SwitchBack: "Time", AlertAt: config.Duration(5 * time.Minute), AlertRTTTL: "beep",
	}}
	render := func(target, now time.Time) (shown bool, other []string) {
		for _, m := range a.Render(fake{"sensor.bus": target.Format(time.RFC3339)}, now) {
			if m.Topic == tp.app("bus333") {
				shown = m.Payload != nil
			} else {
				other = append(other, m.Topic+" "+string(m.Payload))
			}
		}
		return
	}
	// Disarmed: hidden, no switch or sound even inside the windows.
	if shown, other := render(arrival, arrival.Add(-4*time.Minute)); shown || len(other) != 0 {
		t.Fatalf("disarmed: shown=%v other=%v", shown, other)
	}
	a.SetArmed(true, arrival.Add(-30*time.Minute))
	if shown, other := render(arrival, arrival.Add(-3*time.Minute)); !shown || len(other) != 2 {
		t.Fatalf("armed at 3 min: want shown + switch + sound, got shown=%v %v", shown, other)
	}
	render(arrival, arrival.Add(10*time.Second))
	if shown, _ := render(next, arrival.Add(90*time.Second)); !shown { // counting up after feed jump
		t.Fatalf("not shown while counting up")
	}
	shown, other := render(next, arrival.Add(2*time.Minute+time.Second))
	want := []string{`awtrix/cmd/apps/switch {"fast":true,"name":"Time"}`, "awtrix-bridge/bus333/armed off"}
	if shown || len(other) != 2 || other[0] != want[0] || other[1] != want[1] {
		t.Fatalf("after hold: shown=%v other=%v, want hidden + %v", shown, other, want)
	}
	if a.Armed() {
		t.Errorf("should have disarmed itself")
	}

	// Arming while a bus that already arrived is still counting up must
	// wait for the next bus instead of disarming right away.
	a.SetArmed(true, next.Add(30*time.Second))
	render(next, next.Add(31*time.Second))
	render(next.Add(2*time.Hour), next.Add(3*time.Minute))
	if !a.Armed() {
		t.Errorf("armed during count-up of an old bus: disarmed too early")
	}
}

func TestChime(t *testing.T) {
	a := &chimeApp{cfg: config.Chime{QuietEntity: "quiet"}, tune: "hour", topic: "snd"}
	loud, quiet := fake{"quiet": "off"}, fake{"quiet": "on"}
	at := func(h, m, s int) time.Time { return time.Date(2026, 11, 8, h, m, s, 0, time.Local) }
	count := func(st States, from, to time.Time) (n int) {
		for now := from; !now.After(to); now = now.Add(time.Second) {
			n += len(a.Render(st, now))
		}
		return
	}
	if n := count(loud, at(9, 41, 0), at(9, 59, 59)); n != 0 {
		t.Errorf("chimed %d times after start, before the hour", n)
	}
	if n := count(loud, at(10, 0, 0), at(10, 30, 0)); n != 1 {
		t.Errorf("10:00: want 1 chime, got %d", n)
	}
	if n := count(quiet, at(10, 59, 50), at(11, 0, 30)); n != 0 {
		t.Errorf("quiet time: want silence, got %d", n)
	}
	// Quiet time switching on exactly at the hour: the chime waits a few
	// seconds, so it sees the new state.
	a.Render(loud, at(11, 59, 59))
	a.Render(loud, at(12, 0, 0))
	if n := count(quiet, at(12, 0, 1), at(12, 0, 30)); n != 0 {
		t.Errorf("quiet from 12:00:00: want silence, got %d", n)
	}
	// A tick that arrives well after the hour (bridge was down) stays silent.
	if n := len(a.Render(loud, at(13, 5, 0))); n != 0 {
		t.Errorf("late tick chimed")
	}
	if n := count(loud, at(13, 59, 59), at(14, 0, 10)); n != 1 {
		t.Errorf("14:00: want 1 chime, got %d", n)
	}
}
