package apps

import (
	"math"
	"strconv"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

// numeric returns an entity's state as a number; false while it is
// missing, unavailable or not numeric.
func numeric(st States, id string) (float64, bool) {
	s, ok := st.Get(id)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(s.State, 64)
	if err != nil || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// isOn reports whether a binary entity is on; an empty id is never on.
func isOn(st States, id string) bool {
	if id == "" {
		return false
	}
	s, ok := st.Get(id)
	return ok && s.State == "on"
}

func pickColor(v float64, fallback string, th []config.Threshold) string {
	for _, t := range th {
		if t.Below == nil || v < *t.Below {
			return t.Color
		}
	}
	return fallback
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
