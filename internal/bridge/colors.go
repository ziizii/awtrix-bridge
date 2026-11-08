package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

// Colors the admin UI can change. The clock's own colors (time, date,
// weekday bar) are its settings: they are sent to the clock, which keeps
// them and reports them back. Timer and app colors live in the config
// file and are changed with line edits, like the melodies.

// ColorField is one editable color.
type ColorField struct {
	Key   string `json:"key"` // what SetColor takes
	Label string `json:"label"`
	Value string `json:"value"` // "#RRGGBB"; "" = the clock's default text color
}

// AppColors are the editable colors and icons of one configured app.
type AppColors struct {
	App    string       `json:"app"`
	Fields []ColorField `json:"fields"`
	Icons  []ColorField `json:"icons"` // Value is a LaMetric icon id, "" = none
}

type ColorsStatus struct {
	Clock []ColorField `json:"clock"`
	Timer []ColorField `json:"timer"`
	Apps  []AppColors  `json:"apps"`
}

// Color targets of SetColor.
const (
	ColorClock = "clock"
	ColorTimer = "timer"
	ColorApp   = "app"
)

// clockColors are the clock settings offered, as "key" or "object.key".
// The ones that may be empty fall back to the clock's text color.
var clockColors = []struct {
	key, label string
	optional   bool
}{
	{"timeColor", "Time", true},
	{"dateColor", "Date", true},
	{"textColor", "Default text", false},
	{"weekdayBar.activeColor", "Today", false},
	{"weekdayBar.inactiveColor", "Other days", false},
	{"weekdayBar.weekendActiveColor", "Today, weekend", false},
	{"weekdayBar.weekendInactiveColor", "Weekend days", false},
}

var timerColors = []struct{ key, label string }{
	{"color", "Running"},
	{"done_color", "Time's up"},
	{"progress_color", "Progress bar"},
	{"track_color", "Bar track"},
}

func (i *instance) colorsStatus() ColorsStatus {
	st := ColorsStatus{Clock: []ColorField{}, Timer: []ColorField{}, Apps: []AppColors{}}

	var clock map[string]any
	if raw := i.clockSettings(); raw != nil {
		_ = json.Unmarshal(raw, &clock) // nil map: the clock has not reported yet
	}
	if clock != nil {
		for _, c := range clockColors {
			v := clock
			path := strings.Split(c.key, ".")
			for _, p := range path[:len(path)-1] {
				v, _ = v[p].(map[string]any)
			}
			s, _ := v[path[len(path)-1]].(string)
			st.Clock = append(st.Clock, ColorField{Key: c.key, Label: c.label, Value: s})
		}
	}

	t := i.cfg.Timer
	for _, c := range timerColors {
		v := map[string]string{"color": t.Color, "done_color": t.DoneColor, "progress_color": t.ProgressColor, "track_color": t.TrackColor}[c.key]
		st.Timer = append(st.Timer, ColorField{Key: c.key, Label: c.label, Value: v})
	}

	for _, a := range i.cfg.Apps {
		st.Apps = append(st.Apps, AppColors{App: a.Name, Fields: appColorFields(a), Icons: appIconFields(a)})
	}
	return st
}

// appColorFields lists an app's colors: surplus has two, an app with a
// colors list has one per threshold, any other app has one.
func appColorFields(a config.App) []ColorField {
	if a.Type == config.TypeSurplus {
		return []ColorField{
			{Key: "positive_color", Label: "Surplus", Value: a.PositiveColor},
			{Key: "negative_color", Label: "Deficit", Value: a.NegativeColor},
		}
	}
	if len(a.Colors) == 0 {
		return []ColorField{{Key: "color", Label: "Text", Value: a.Color}}
	}
	unit := ""
	if a.Type == config.TypeCountdown {
		unit = " min" // countdown thresholds are minutes remaining
	}
	out := []ColorField{}
	for n, th := range a.Colors {
		label := "Otherwise"
		if th.Below != nil {
			label = "Below " + strconv.FormatFloat(*th.Below, 'f', -1, 64) + unit
		}
		out = append(out, ColorField{Key: fmt.Sprintf("colors.%d", n), Label: label, Value: th.Color})
	}
	return out
}

// appIconFields lists an app's icons: surplus also has one for when the
// panels produce nothing.
func appIconFields(a config.App) []ColorField {
	out := []ColorField{{Key: "icon", Label: "Icon", Value: a.Icon}}
	if a.Type == config.TypeSurplus {
		out[0].Label = "Day icon"
		out = append(out, ColorField{Key: "night_icon", Label: "Night icon", Value: a.NightIcon})
	}
	return out
}

// SetAppIcon sets an app's icon (key "icon", or "night_icon" for
// surplus) to a LaMetric icon id, uploading it to the clock first if it
// is missing there; "" removes it.
func (s *Supervisor) SetAppIcon(ctx context.Context, app, key, id string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	var fields []ColorField
	for _, a := range inst.cfg.Apps {
		if a.Name == app {
			fields = appIconFields(a)
		}
	}
	if !hasKey(fields, key) {
		return fmt.Errorf("app %q has no icon %q", app, key)
	}
	if err := inst.checkIcon(ctx, id); err != nil {
		return err
	}
	return s.edit(ctx, func(raw []byte) ([]byte, error) {
		return config.SetApp(raw, app, []config.Field{{Key: key, Value: strconv.Quote(id)}})
	})
}

// SetColor changes one color: of the clock (target "clock", keys from
// clockColors), the timer ("timer"), or an app ("app" with its name,
// keys from appColorFields).
func (s *Supervisor) SetColor(ctx context.Context, target, app, key, value string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	switch target {
	case ColorClock:
		return inst.setClockColor(key, value)
	case ColorTimer:
		if err := config.CheckColor(value); err != nil {
			return err
		}
		if !hasKey(inst.colorsStatus().Timer, key) {
			return fmt.Errorf("unknown timer color %q", key)
		}
		return s.edit(ctx, func(raw []byte) ([]byte, error) {
			return config.SetSection(raw, "timer", []config.Field{{Key: key, Value: strconv.Quote(value)}}), nil
		})
	case ColorApp:
		if err := config.CheckColor(value); err != nil {
			return err
		}
		var fields []ColorField
		for _, a := range inst.cfg.Apps {
			if a.Name == app {
				fields = appColorFields(a)
			}
		}
		if !hasKey(fields, key) {
			return fmt.Errorf("app %q has no color %q", app, key)
		}
		return s.edit(ctx, func(raw []byte) ([]byte, error) {
			if n, ok := strings.CutPrefix(key, "colors."); ok {
				idx, _ := strconv.Atoi(n)
				return config.SetThresholdColor(raw, app, idx, value)
			}
			return config.SetApp(raw, app, []config.Field{{Key: key, Value: strconv.Quote(value)}})
		})
	}
	return fmt.Errorf("unknown color target %q", target)
}

// setClockColor sends one color setting to the clock; "" resets an
// optional one to the clock's text color.
func (i *instance) setClockColor(key, value string) error {
	var def *struct {
		key, label string
		optional   bool
	}
	for n := range clockColors {
		if clockColors[n].key == key {
			def = &clockColors[n]
		}
	}
	if def == nil {
		return fmt.Errorf("unknown clock color %q", key)
	}
	var v any = value
	if value == "" && def.optional {
		v = nil
	} else if err := config.CheckColor(value); err != nil {
		return err
	}
	payload := map[string]any{}
	if obj, k, nested := strings.Cut(key, "."); nested {
		payload[obj] = map[string]any{k: v}
	} else {
		payload[key] = v
	}
	b, _ := json.Marshal(payload)
	return i.Publish(i.topics.Settings, b, false)
}

func hasKey(fields []ColorField, key string) bool {
	for _, f := range fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

// PreviewColor shows a color on the clock while it is being picked,
// without saving it: clock colors are sent to the clock (the picker sends
// the old one back on cancel), timer and app colors are shown by the
// running app until EndPreview or SetColor.
func (s *Supervisor) PreviewColor(target, app, key, value string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	switch target {
	case ColorClock:
		return inst.setClockColor(key, value)
	case ColorTimer:
		app = config.TimerApp
		if !hasKey(inst.colorsStatus().Timer, key) {
			return fmt.Errorf("unknown timer color %q", key)
		}
	case ColorApp:
		var fields []ColorField
		for _, a := range inst.cfg.Apps {
			if a.Name == app {
				fields = appColorFields(a)
			}
		}
		if !hasKey(fields, key) {
			return fmt.Errorf("app %q has no color %q", app, key)
		}
	default:
		return fmt.Errorf("unknown color target %q", target)
	}
	if err := config.CheckColor(value); err != nil {
		return err
	}
	inst.eng.PreviewColor(app, key, value)
	return nil
}

// EndPreview drops previewed timer or app colors (the picker was
// cancelled); clock colors need nothing.
func (s *Supervisor) EndPreview(target, app string) error {
	inst, err := s.current()
	if err != nil {
		return err
	}
	switch target {
	case ColorTimer:
		inst.eng.EndPreview(config.TimerApp)
	case ColorApp:
		inst.eng.EndPreview(app)
	}
	return nil
}
