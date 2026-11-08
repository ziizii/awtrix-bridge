package apps

import (
	"strconv"
	"strings"

	"github.com/ziizii/awtrix-bridge/internal/config"
)

// Previewable apps can show colors tried in the admin UI's picker on the
// clock before they are saved. Keys are the config keys ("color",
// "colors.1", "positive_color", "done_color", ...). Saving reloads the
// config, which drops the preview along with the old apps.
type Previewable interface {
	App
	SetPreview(key, color string)
	ClearPreview()
}

// colorPreview is embedded in apps that support previews.
type colorPreview struct {
	colors  map[string]string
	lastKey string // the color being picked, e.g. to show the timer's end state
}

func (p *colorPreview) SetPreview(key, color string) {
	if p.colors == nil {
		p.colors = map[string]string{}
	}
	p.colors[key] = color
	p.lastKey = key
}

func (p *colorPreview) ClearPreview() { *p = colorPreview{} }

func (p *colorPreview) previewing() bool { return len(p.colors) > 0 }

// col returns the previewed color for key, else def.
func (p *colorPreview) col(key, def string) string {
	if c, ok := p.colors[key]; ok {
		return c
	}
	return def
}

// thresholds returns th with previewed "colors.N" entries applied.
func (p *colorPreview) thresholds(th []config.Threshold) []config.Threshold {
	if len(p.colors) == 0 {
		return th
	}
	out := append([]config.Threshold(nil), th...)
	for k, c := range p.colors {
		if n, ok := strings.CutPrefix(k, "colors."); ok {
			if i, err := strconv.Atoi(n); err == nil && i >= 0 && i < len(out) {
				out[i].Color = c
			}
		}
	}
	return out
}
