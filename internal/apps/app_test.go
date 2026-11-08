package apps

import (
	"testing"
	"time"

	"encoding/json"
	"github.com/ziizii/awtrix-bridge/internal/awtrix"
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

var topics = awtrix.NewTopics(config.Awtrix{
	AppTopic: "awtrix/cmd/apps/pushed/{name}", SettingsTopic: "s", SoundTopic: "snd",
	SwitchTopic: "awtrix/cmd/apps/switch", ArmTopic: "awtrix-bridge/{name}/armed",
})

func TestCarryKeepsSwitchState(t *testing.T) {
	cfg := &config.Config{
		Awtrix: config.Awtrix{AppTopic: "awtrix/cmd/apps/pushed/{name}", SwitchTopic: "awtrix/cmd/apps/switch"},
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
