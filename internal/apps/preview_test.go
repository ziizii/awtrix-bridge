package apps

import (
	"testing"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

func TestColorPreview(t *testing.T) {
	below := 100.0
	v := &valueApp{topic: "t", cfg: config.App{Entity: "sensor.x", Format: "%.0f", Colors: []config.Threshold{{Below: &below, Color: "#00FF00"}, {Color: "#FF0000"}}}}
	st := fake{"sensor.x": "50"}
	color := func(a App) any { return decode(t, a.Render(st, time.Time{})[0].Payload)["textColor"] }
	if color(v) != "#00FF00" {
		t.Fatalf("base color %v", color(v))
	}
	v.SetPreview("colors.0", "#123456")
	if color(v) != "#123456" {
		t.Errorf("threshold preview %v", color(v))
	}
	if v.cfg.Colors[0].Color != "#00FF00" {
		t.Errorf("preview changed the config")
	}
	v.ClearPreview()
	if color(v) != "#00FF00" {
		t.Errorf("after clear %v", color(v))
	}

	// Surplus shows the deficit color while it is being picked.
	s := &surplusApp{topic: "s", cfg: config.App{Production: "p", Consumption: "c", PositiveColor: "#00FF00", NegativeColor: "#FF4400"}}
	st = fake{"p": "2000", "c": "500"}
	s.SetPreview("negative_color", "#0000FF")
	if color(s) != "#0000FF" {
		t.Errorf("deficit preview %v", color(s))
	}
}

func TestTimerPreviewSample(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	a := &timerApp{topics: topics, cfg: config.Timer{Color: "#FFFFFF", DoneColor: "#FF0000", TrackColor: "#303030"}}
	if msgs := a.Render(nil, now); msgs[0].Payload != nil {
		t.Fatalf("idle timer shown: %s", msgs[0].Payload)
	}
	a.SetPreview("track_color", "#00FF00")
	p := decode(t, a.Render(nil, now)[0].Payload)
	if p["text"] != "2:05" || p["progressTrackColor"] != "#00FF00" {
		t.Errorf("running sample %v", p)
	}
	a.SetPreview("done_color", "#0000FF")
	if p := decode(t, a.Render(nil, now)[0].Payload); p["text"] != "+0:12" || p["textColor"] != "#0000FF" {
		t.Errorf("done sample %v", p)
	}
	a.ClearPreview()
	if msgs := a.Render(nil, now); msgs[0].Payload != nil {
		t.Errorf("sample stayed after the preview")
	}
}
