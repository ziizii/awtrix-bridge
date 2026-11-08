// Package bridge runs the HA -> clock pipeline and swaps it for a new one
// when the configuration changes.
//
//   - supervisor.go: owns the config file and the running instance; reloads.
//   - instance.go:   one running pipeline (MQTT, HA, engine) for one config.
//   - actions.go:    what the admin UI can do (switch, arm, pick melodies...).
//   - status.go:     what the admin UI shows.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

// Supervisor owns the config file and the running instance.
type Supervisor struct {
	path   string
	log    *slog.Logger
	reqs   chan reloadReq
	fileMu sync.Mutex // serializes config file edits

	mu      sync.Mutex // guards the fields below
	cur     *instance
	started time.Time
	loaded  time.Time
}

type reloadReq struct {
	cfg  *config.Config
	done chan struct{}
}

func New(path string, log *slog.Logger) *Supervisor {
	return &Supervisor{path: path, log: log, reqs: make(chan reloadReq)}
}

// Run starts the bridge and serves reloads until ctx is cancelled. A config
// that fails to load at startup is fatal.
func (s *Supervisor) Run(ctx context.Context) error {
	cfg, err := config.Load(s.path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	inst, err := start(ctx, cfg, nil, s.log)
	if err != nil {
		return err
	}
	s.setCurrent(inst, true)
	for {
		select {
		case <-ctx.Done():
			inst.stop()
			return nil
		case r := <-s.reqs:
			inst = s.swap(ctx, inst, r.cfg)
			s.setCurrent(inst, false)
			close(r.done)
		}
	}
}

// swap stops old and starts a pipeline for cfg, handing over runtime
// state. Configs are validated before they get here, so a failure is
// unexpected; the old config is restarted then.
func (s *Supervisor) swap(ctx context.Context, old *instance, cfg *config.Config) *instance {
	old.removeDropped(cfg)
	old.stop()
	next, err := start(ctx, cfg, old.apps, s.log)
	if err != nil {
		s.log.Error("reload failed, restarting previous config", "err", err)
		next, _ = start(ctx, old.cfg, old.apps, s.log)
		return next
	}
	s.log.Info("config reloaded", "apps", len(cfg.Apps))
	return next
}

func (s *Supervisor) setCurrent(inst *instance, first bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur, s.loaded = inst, time.Now()
	if first {
		s.started = s.loaded
	}
}

// current returns the running instance.
func (s *Supervisor) current() (*instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return nil, errors.New("bridge not running")
	}
	return s.cur, nil
}

// Raw returns the config file as stored (with ${VAR} references unexpanded).
func (s *Supervisor) Raw() ([]byte, error) { return os.ReadFile(s.path) }

// Check parses and validates raw config without applying it.
func (s *Supervisor) Check(raw []byte) error {
	_, err := check(raw)
	return err
}

// Apply validates raw, writes it to the config file and reloads.
func (s *Supervisor) Apply(ctx context.Context, raw []byte) error {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	return s.apply(ctx, raw)
}

// Reload re-reads the config file and applies it.
func (s *Supervisor) Reload(ctx context.Context) error {
	raw, err := s.Raw()
	if err != nil {
		return err
	}
	cfg, err := check(raw)
	if err != nil {
		return err
	}
	return s.reload(ctx, cfg)
}

// edit changes the config file with fn (a config.Set* line edit) and
// applies the result; the file lock covers the read-modify-write.
func (s *Supervisor) edit(ctx context.Context, fn func(raw []byte) ([]byte, error)) error {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	raw, err := s.Raw()
	if err != nil {
		return err
	}
	out, err := fn(raw)
	if err != nil {
		return err
	}
	return s.apply(ctx, out)
}

// apply needs fileMu held.
func (s *Supervisor) apply(ctx context.Context, raw []byte) error {
	cfg, err := check(raw)
	if err != nil {
		return err
	}
	// Written in place: the file is usually a single-file bind mount,
	// which cannot be replaced by rename.
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", s.path, err)
	}
	return s.reload(ctx, cfg)
}

func (s *Supervisor) reload(ctx context.Context, cfg *config.Config) error {
	r := reloadReq{cfg: cfg, done: make(chan struct{})}
	select {
	case s.reqs <- r:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// check parses raw and makes sure an instance could be started from it.
func check(raw []byte) (*config.Config, error) {
	cfg, err := config.Parse(raw)
	if err != nil {
		return nil, err
	}
	if _, err := apps.Build(cfg); err != nil {
		return nil, err
	}
	if _, err := ha.New(cfg.HomeAssistant.URL, cfg.HomeAssistant.Token, slog.Default()); err != nil {
		return nil, fmt.Errorf("homeassistant.url: %w", err)
	}
	return cfg, nil
}
