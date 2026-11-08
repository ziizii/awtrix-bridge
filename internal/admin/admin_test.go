package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ziizii/awtrix-bridge/internal/bridge"
)

type fakeBackend struct {
	applied string
	armed   map[string]bool
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

	if w := do(h, "GET", "/", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "<title>Clock Admin</title>") {
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
}
