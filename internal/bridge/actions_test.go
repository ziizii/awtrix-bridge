package bridge

import (
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
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

func TestAppColorFields(t *testing.T) {
	two, six := 2.0, 6.0
	bus := config.App{Name: "bus", Type: config.TypeCountdown, Colors: []config.Threshold{{Below: &two, Color: "#F00"}, {Below: &six, Color: "#FA0"}, {Color: "#0F0"}}}
	got := appColorFields(bus)
	if len(got) != 3 || got[0].Label != "Below 2 min" || got[2].Label != "Otherwise" || got[1].Key != "colors.1" {
		t.Errorf("countdown: %+v", got)
	}
	if got := appColorFields(config.App{Type: config.TypeSurplus, PositiveColor: "#0F0"}); len(got) != 2 || got[0].Value != "#0F0" {
		t.Errorf("surplus: %+v", got)
	}
	if got := appColorFields(config.App{Type: config.TypeValue, Color: "#FC0"}); len(got) != 1 || got[0].Key != "color" {
		t.Errorf("value: %+v", got)
	}
}

func TestAppIconFields(t *testing.T) {
	if got := appIconFields(config.App{Type: config.TypeSurplus, Icon: "1", NightIcon: "2"}); len(got) != 2 || got[1].Key != "night_icon" || got[1].Value != "2" {
		t.Errorf("surplus: %+v", got)
	}
	if got := appIconFields(config.App{Type: config.TypeValue, Icon: "6401"}); len(got) != 1 || got[0].Key != "icon" || got[0].Value != "6401" {
		t.Errorf("value: %+v", got)
	}
}
