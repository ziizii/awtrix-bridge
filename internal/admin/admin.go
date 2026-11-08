// Package admin serves the web UI: the page in web/ and a small JSON API.
// It has no authentication: expose it on a trusted network only.
//
// Adding an action: add a method to Backend (implemented by
// bridge.Supervisor), a request type and one route line in Handler.
package admin

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/bridge"
)

//go:embed web
var webFS embed.FS

// Backend is what the UI controls; *bridge.Supervisor implements it.
type Backend interface {
	Status() bridge.Status
	Raw() ([]byte, error)
	Check(raw []byte) error
	Apply(ctx context.Context, raw []byte) error
	Reload(ctx context.Context) error
	Switch(app string) error
	PlayMelody(name string) error
	SetArmed(app string, on bool) error
	SetChime(ctx context.Context, melody string) error
	SetAlert(ctx context.Context, app string, minutes int, melody string) error
}

// Request bodies.
type (
	yamlReq   struct{ YAML string }
	appReq    struct{ App string }
	melodyReq struct{ Melody string }
	armReq    struct {
		App   string
		Armed bool
	}
	alertReq struct {
		App     string
		Minutes int
		Melody  string
	}
)

// Handler returns the admin HTTP handler.
func Handler(b Backend) http.Handler {
	mux := http.NewServeMux()
	web, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", noCache(http.FileServerFS(web)))

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusOK, b.Status())
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, _ *http.Request) {
		raw, err := b.Raw()
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		reply(w, http.StatusOK, map[string]string{"yaml": string(raw)})
	})

	mux.Handle("POST /api/check", action(func(_ context.Context, r yamlReq) error { return b.Check([]byte(r.YAML)) }))
	mux.Handle("PUT /api/config", action(func(ctx context.Context, r yamlReq) error { return b.Apply(ctx, []byte(r.YAML)) }))
	mux.Handle("POST /api/reload", action(func(ctx context.Context, _ struct{}) error { return b.Reload(ctx) }))
	mux.Handle("POST /api/switch", action(func(_ context.Context, r appReq) error { return b.Switch(r.App) }))
	mux.Handle("POST /api/arm", action(func(_ context.Context, r armReq) error { return b.SetArmed(r.App, r.Armed) }))
	mux.Handle("POST /api/melody/play", action(func(_ context.Context, r melodyReq) error { return b.PlayMelody(r.Melody) }))
	mux.Handle("POST /api/chime", action(func(ctx context.Context, r melodyReq) error { return b.SetChime(ctx, r.Melody) }))
	mux.Handle("POST /api/alert", action(func(ctx context.Context, r alertReq) error {
		return b.SetAlert(ctx, r.App, r.Minutes, r.Melody)
	}))
	return mux
}

const (
	maxBody       = 1 << 20 // the config editor sends the whole file
	actionTimeout = 30 * time.Second
)

// action adapts fn to a JSON endpoint: it requires a JSON content type,
// decodes the body into T (field names match case-insensitively), and
// replies {"ok":true} or {"error":"..."}.
func action[T any](fn func(context.Context, T) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A JSON content type makes cross-site form posts impossible
		// (browsers preflight it), which is the only guard this UI has.
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			fail(w, http.StatusUnsupportedMediaType, errors.New("Content-Type must be application/json"))
			return
		}
		var in T
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&in); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), actionTimeout)
		defer cancel()
		if err := fn(ctx, in); err != nil {
			fail(w, http.StatusUnprocessableEntity, err)
			return
		}
		reply(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

// noCache makes browsers revalidate the page and scripts after a deploy.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	reply(w, code, map[string]string{"error": err.Error()})
}
