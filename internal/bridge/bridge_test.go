package bridge

import (
	"strings"
	"testing"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

const base = `homeassistant: {url: "http://ha", token: t}
mqtt: {broker: "tcp://b:1883"}
`

func TestSetChimeMelody(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"replace custom rtttl": {
			in:   base + "chime:                # hourly\n  rtttl: \"hour:d=4:c\"\n  quiet_entity: q   # silent\n\napps: []\n",
			want: "  melody: ding\n  # rtttl: \"hour:d=4:c\"\n  quiet_entity: q   # silent\n",
		},
		"change melody": {
			in:   base + "chime:\n  melody: tick\n  quiet_entity: q\napps: []\n",
			want: "chime:\n  melody: ding\n  quiet_entity: q\napps: []\n",
		},
		"add block": {
			in:   base + "apps: []\n",
			want: "chime:\n  melody: ding\n\napps: []\n",
		},
	}
	for name, c := range cases {
		out := string(setChimeMelody([]byte(c.in), "ding"))
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: got\n%s", name, out)
			continue
		}
		cfg, err := config.Parse([]byte(out))
		if err != nil || cfg.Chime == nil || cfg.Chime.Tune() == "" || cfg.Chime.Melody != "ding" {
			t.Errorf("%s: parsed %+v, %v", name, cfg, err)
		}
	}
}
