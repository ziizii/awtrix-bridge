// Package admin serves a small web UI to view the bridge and edit its YAML
// config. It has no authentication: expose it on a trusted network only.
package admin

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/bridge"
)

//go:embed index.html
var indexHTML []byte

// Backend is what the UI controls; *bridge.Supervisor implements it.
type Backend interface {
	Raw() ([]byte, error)
	Check(raw []byte) error
	Apply(ctx context.Context, raw []byte) error
	Reload(ctx context.Context) error
	Status() bridge.Status
	SetArmed(name string, on bool) error
	Switch(name string) error
}

const maxConfig = 1 << 20

// Handler returns the admin HTTP handler.
func Handler(b Backend) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(indexHTML)
	})
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
	mux.HandleFunc("POST /api/check", func(w http.ResponseWriter, r *http.Request) {
		raw, ok := body(w, r)
		if !ok {
			return
		}
		if err := b.Check(raw); err != nil {
			fail(w, http.StatusUnprocessableEntity, err)
			return
		}
		reply(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		raw, ok := body(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := b.Apply(ctx, raw); err != nil {
			fail(w, http.StatusUnprocessableEntity, err)
			return
		}
		reply(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/reload", func(w http.ResponseWriter, r *http.Request) {
		if !isJSON(r) {
			fail(w, http.StatusUnsupportedMediaType, errJSON)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := b.Reload(ctx); err != nil {
			fail(w, http.StatusUnprocessableEntity, err)
			return
		}
		reply(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/arm", func(w http.ResponseWriter, r *http.Request) {
		if !isJSON(r) {
			fail(w, http.StatusUnsupportedMediaType, errJSON)
			return
		}
		var in struct {
			App   string `json:"app"`
			Armed bool   `json:"armed"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		if err := b.SetArmed(in.App, in.Armed); err != nil {
			fail(w, http.StatusUnprocessableEntity, err)
			return
		}
		reply(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/switch", func(w http.ResponseWriter, r *http.Request) {
		if !isJSON(r) {
			fail(w, http.StatusUnsupportedMediaType, errJSON)
			return
		}
		var in struct {
			App string `json:"app"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		if err := b.Switch(in.App); err != nil {
			fail(w, http.StatusUnprocessableEntity, err)
			return
		}
		reply(w, http.StatusOK, map[string]bool{"ok": true})
	})
	return mux
}

type jsonErr string

func (e jsonErr) Error() string { return string(e) }

// Requiring a JSON content type makes cross-site form posts impossible
// (browsers preflight it), which is the only guard this UI has.
const errJSON = jsonErr("Content-Type must be application/json")

func isJSON(r *http.Request) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return mt == "application/json"
}

func body(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if !isJSON(r) {
		fail(w, http.StatusUnsupportedMediaType, errJSON)
		return nil, false
	}
	var in struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxConfig)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return nil, false
	}
	return []byte(in.YAML), true
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
