package bridge

import (
	"testing"
	"time"
)

func TestNextTimeOfDay(t *testing.T) {
	prague, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		t.Skip(err)
	}
	now := time.Date(2026, 11, 8, 14, 30, 0, 0, prague)
	cases := map[string]time.Time{
		"14:45": time.Date(2026, 11, 8, 14, 45, 0, 0, prague),
		"14:30": time.Date(2026, 11, 9, 14, 30, 0, 0, prague), // now: the next one
		"07:00": time.Date(2026, 11, 9, 7, 0, 0, 0, prague),
	}
	for in, want := range cases {
		if got, err := nextTimeOfDay(in, now); err != nil || !got.Equal(want) {
			t.Errorf("%s: got %v %v, want %v", in, got, err, want)
		}
	}
	// Across the end of daylight saving time the wall clock time still matches.
	dst := time.Date(2026, 10, 24, 22, 0, 0, 0, prague)
	if got, _ := nextTimeOfDay("21:00", dst); got.Hour() != 21 || got.Day() != 25 {
		t.Errorf("dst: %v", got)
	}
	for _, bad := range []string{"", "25:00", "noon", "14:30:00"} {
		if _, err := nextTimeOfDay(bad, now); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
