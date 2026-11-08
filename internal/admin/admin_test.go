package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ziizii/awtrix-bridge/internal/bridge"
)

type fakeBackend struct {
	applied  string
	armed    map[string]bool
	switched string
	played   string
	chime    string
	alert    string
	timer    string
}

func (f *fakeBackend) StartTimer(seconds int, at string) error {
	f.timer = fmt.Sprintf("start %d %s", seconds, at)
	return nil
}
func (f *fakeBackend) ExtendTimer(seconds int) error {
	f.timer = fmt.Sprintf("extend %d", seconds)
	return nil
}
func (f *fakeBackend) SetColor(_ context.Context, target, app, key, value string) error {
	f.timer = fmt.Sprintf("color %s %s %s %s", target, app, key, value)
	return nil
}
func (f *fakeBackend) CancelTimer() error { f.timer = "cancel"; return nil }
func (f *fakeBackend) SetTimerMelody(_ context.Context, melody string) error {
	f.timer = "melody " + melody
	return nil
}

func (f *fakeBackend) PlayMelody(name string) error { f.played = name; return nil }
func (f *fakeBackend) SetChime(_ context.Context, name string) error {
	f.chime = name
	return nil
}

func (f *fakeBackend) SetAlert(_ context.Context, app string, minutes int, melody string) error {
	f.alert = fmt.Sprintf("%s %d %s", app, minutes, melody)
	return nil
}

func (f *fakeBackend) Switch(name string) error {
	f.switched = name
	return nil
}

func (f *fakeBackend) Raw() ([]byte, error) { return []byte("apps: []\n"), nil }
func (f *fakeBackend) Check(raw []byte) error {
	if strings.Contains(string(raw), "bad") {
		return errors.New("bad config")
	}
	return nil
}
func (f *fakeBackend) Apply(_ context.Context, raw []byte) error {
	if err := f.Check(raw); err != nil {
		return err
	}
	f.applied = string(raw)
	return nil
}
func (f *fakeBackend) Reload(context.Context) error { return nil }
func (f *fakeBackend) Status() bridge.Status        { return bridge.Status{} }
func (f *fakeBackend) SetArmed(name string, on bool) error {
	f.armed[name] = on
	return nil
}

func do(h http.Handler, method, path, ctype, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAdmin(t *testing.T) {
	b := &fakeBackend{armed: map[string]bool{}}
	h := Handler(b)

	if w := do(h, "GET", "/", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `src="js/main.js"`) {
		t.Errorf("index: %d", w.Code)
	}
	// A cross-site form post (no JSON content type) must be rejected.
	if w := do(h, "PUT", "/api/config", "text/plain", `{"yaml":"x: 1"}`); w.Code != http.StatusUnsupportedMediaType || b.applied != "" {
		t.Errorf("text/plain accepted: %d", w.Code)
	}
	if w := do(h, "PUT", "/api/config", "application/json", `{"yaml":"bad"}`); w.Code != http.StatusUnprocessableEntity || b.applied != "" {
		t.Errorf("invalid config: %d %s", w.Code, w.Body)
	}
	if w := do(h, "PUT", "/api/config", "application/json", `{"yaml":"x: 1"}`); w.Code != 200 || b.applied != "x: 1" {
		t.Errorf("apply: %d %q", w.Code, b.applied)
	}
	if w := do(h, "POST", "/api/arm", "application/json", `{"app":"bus333","armed":true}`); w.Code != 200 || !b.armed["bus333"] {
		t.Errorf("arm: %d %v", w.Code, b.armed)
	}
	if w := do(h, "POST", "/api/switch", "text/plain", `{"app":"temp_out"}`); w.Code != http.StatusUnsupportedMediaType || b.switched != "" {
		t.Errorf("switch accepted text/plain: %d", w.Code)
	}
	if w := do(h, "POST", "/api/switch", "application/json", `{"app":"temp_out"}`); w.Code != 200 || b.switched != "temp_out" {
		t.Errorf("switch: %d %q", w.Code, b.switched)
	}
	if w := do(h, "POST", "/api/melody/play", "application/json", `{"melody":"ding"}`); w.Code != 200 || b.played != "ding" {
		t.Errorf("play: %d %q", w.Code, b.played)
	}
	if w := do(h, "POST", "/api/chime", "application/json", `{"melody":"off"}`); w.Code != 200 || b.chime != "off" {
		t.Errorf("set chime: %d %q", w.Code, b.chime)
	}
	if w := do(h, "POST", "/api/alert", "application/json", `{"app":"bus333","minutes":7,"melody":"beeps"}`); w.Code != 200 || b.alert != "bus333 7 beeps" {
		t.Errorf("set alert: %d %q", w.Code, b.alert)
	}
	for path, want := range map[string]string{
		`/api/timer {"seconds":300}`:       "start 300 ",
		`/api/timer {"at":"14:30"}`:        "start 0 14:30",
		`/api/timer/extend {"seconds":60}`: "extend 60",
		`/api/timer/cancel {}`:             "cancel",
		`/api/color {"target":"app","app":"solar","key":"color","value":"#FFCC00"}`: "color app solar color #FFCC00",
		`/api/timer/melody {"melody":"ding"}`:                                       "melody ding",
	} {
		p, body, _ := strings.Cut(path, " ")
		if w := do(h, "POST", p, "application/json", body); w.Code != 200 || b.timer != want {
			t.Errorf("%s: %d %q, want %q", path, w.Code, b.timer, want)
		}
	}
}
