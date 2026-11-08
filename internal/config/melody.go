package config

import "fmt"

// Melody is a built-in tune, selectable in the admin UI for the hourly
// chime and countdown alerts.
type Melody struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	RTTTL string `json:"-"`
}

// MelodyOff disables a chime or alert where a melody name is expected.
const MelodyOff = "off"

// Melodies lists the built-in tunes, quietest first.
var Melodies = []Melody{
	{"tick", "Tick: one short blip", "tick:d=32,o=6,b=200:c"},
	{"ding", "Ding: a single note", "ding:d=16,o=6,b=120:e"},
	{"dingdong", "Ding-dong: two notes", "dingdong:d=8,o=6,b=140:e,c"},
	{"chirp", "Chirp: a quick rising trill", "chirp:d=32,o=7,b=200:c,e,g"},
	{"twotone", "Two-tone: low then high", "two:d=4,o=5,b=200:c,e"},
	{"beeps", "Beeps: three quick beeps", "beeps:d=16,o=6,b=180:c,p,c,p,c"},
	{"westminster", "Westminster: four bell notes", "hour:d=4,o=5,b=140:e6,c6,d6,2g"},
}

// MelodyRTTTL returns the tune of a built-in melody.
func MelodyRTTTL(name string) (string, bool) {
	for _, m := range Melodies {
		if m.Name == name {
			return m.RTTTL, true
		}
	}
	return "", false
}

// tune resolves a melody setting: a built-in name, "off", or empty to use
// the custom RTTTL.
func tune(melody, custom string) string {
	switch melody {
	case MelodyOff:
		return ""
	case "":
		return custom
	}
	t, _ := MelodyRTTTL(melody)
	return t
}

func checkMelody(name string) error {
	if name == "" || name == MelodyOff {
		return nil
	}
	if _, ok := MelodyRTTTL(name); !ok {
		return fmt.Errorf("unknown melody %q", name)
	}
	return nil
}
