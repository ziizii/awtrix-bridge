// Package bridge runs the HA -> AWTRIX pipeline and swaps it for a new one
// when the configuration is reloaded.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/engine"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

// Supervisor owns the config file and the running instance.
type Supervisor struct {
	path string
	log  *slog.Logger
	reqs chan reloadReq

	mu      sync.Mutex
	cur     *instance
	started time.Time
	loaded  time.Time
	fileMu  sync.Mutex // serializes config file writes
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
	s.mu.Lock()
	s.cur, s.started, s.loaded = inst, time.Now(), time.Now()
	s.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			inst.stop()
			return nil
		case r := <-s.reqs:
			inst.removeDropped(r.cfg)
			inst.stop()
			next, err := start(ctx, r.cfg, inst.apps, s.log)
			if err != nil { // validated before, so unexpected; keep the old config
				s.log.Error("reload failed, restarting previous config", "err", err)
				next, _ = start(ctx, inst.cfg, inst.apps, s.log)
			} else {
				s.log.Info("config reloaded", "apps", len(r.cfg.Apps))
			}
			inst = next
			s.mu.Lock()
			s.cur, s.loaded = inst, time.Now()
			s.mu.Unlock()
			close(r.done)
		}
	}
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
	cfg, err := check(raw)
	if err != nil {
		return err
	}
	s.fileMu.Lock()
	// Written in place: the file is usually a single-file bind mount,
	// which cannot be replaced by rename.
	err = os.WriteFile(s.path, raw, 0o644)
	s.fileMu.Unlock()
	if err != nil {
		return fmt.Errorf("write %s: %w", s.path, err)
	}
	return s.reload(ctx, cfg)
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

// Status describes the running instance.
type Status struct {
	Started time.Time          `json:"started"`
	Loaded  time.Time          `json:"loaded"`
	MQTT    bool               `json:"mqtt"`
	Apps    []engine.Published `json:"apps"`
	Recent  []engine.Published `json:"recent"`
	Arms    []engine.ArmState  `json:"arms"`
	Active  string             `json:"active"` // app on screen, as the clock reports it
	Chime   ChimeStatus        `json:"chime"`
}

// ChimeStatus is the hourly chime choice and the built-in options.
type ChimeStatus struct {
	Melody   string          `json:"melody"` // preset name, "off", or "custom"
	Melodies []config.Melody `json:"melodies"`
	Quiet    *QuietStatus    `json:"quiet,omitempty"`
}

// QuietStatus is the entity that silences the chime, as HA reports it.
type QuietStatus struct {
	Entity string `json:"entity"`
	Name   string `json:"name"`
	On     bool   `json:"on"`
	State  string `json:"state"`
	From   string `json:"from,omitempty"` // "after" attribute of a time-of-day sensor
	To     string `json:"to,omitempty"`   // "before" attribute
}

func (s *Supervisor) Status() Status {
	s.mu.Lock()
	inst, st := s.cur, Status{Started: s.started, Loaded: s.loaded}
	s.mu.Unlock()
	if inst != nil {
		st.MQTT = inst.pub.c.IsConnectionOpen()
		st.Apps, st.Recent = inst.eng.Status()
		st.Arms = inst.eng.Arms()
		st.Active = inst.activeApp()
		st.Chime = ChimeStatus{Melody: "off", Melodies: config.ChimeMelodies}
		if ch := inst.cfg.Chime; ch != nil {
			switch {
			case ch.Melody != "":
				st.Chime.Melody = ch.Melody
			case ch.RTTTL != "":
				st.Chime.Melody = "custom"
			}
			if ch.QuietEntity != "" {
				q := &QuietStatus{Entity: ch.QuietEntity, State: "unknown"}
				if hs, ok := inst.eng.State(ch.QuietEntity); ok {
					q.State, q.On = hs.State, hs.State == "on"
					q.Name, _ = hs.Attributes["friendly_name"].(string)
					q.From, _ = hs.Attributes["after"].(string)
					q.To, _ = hs.Attributes["before"].(string)
				}
				st.Chime.Quiet = q
			}
		}
	}
	return st
}

// PlayMelody plays a built-in chime melody on the clock now (preview).
func (s *Supervisor) PlayMelody(name string) error {
	s.mu.Lock()
	inst := s.cur
	s.mu.Unlock()
	if inst == nil {
		return errors.New("bridge not running")
	}
	tune, ok := config.MelodyRTTTL(name)
	if !ok {
		return fmt.Errorf("unknown melody %q", name)
	}
	p, _ := json.Marshal(map[string]string{"rtttl": tune})
	return inst.pub.Publish(inst.cfg.Awtrix.SoundTopic, p, false)
}

// SetChime selects the hourly chime melody ("off" to silence it) by
// editing chime.melody in the config file and reloading.
func (s *Supervisor) SetChime(ctx context.Context, name string) error {
	raw, err := s.Raw()
	if err != nil {
		return err
	}
	return s.Apply(ctx, setChimeMelody(raw, name))
}

// setChimeMelody sets chime.melody in raw YAML with a line edit, so the
// file's comments and layout stay as they are. A custom rtttl line is
// commented out because melody now takes over.
func setChimeMelody(raw []byte, name string) []byte {
	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "chime:") {
			start = i
			break
		}
	}
	if start < 0 {
		block := []string{"chime:", "  melody: " + name, ""}
		at := len(lines)
		for i, l := range lines {
			if strings.HasPrefix(l, "apps:") {
				at = i
				break
			}
		}
		lines = append(lines[:at], append(block, lines[at:]...)...)
		return []byte(strings.Join(lines, "\n"))
	}
	set := false
	i := start + 1
	for ; i < len(lines); i++ {
		l := lines[i]
		if l != "" && !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "#") {
			break // end of the chime block
		}
		key := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(key, "melody:"):
			lines[i] = "  melody: " + name
			set = true
		case strings.HasPrefix(key, "rtttl:"):
			lines[i] = "  # " + key
		}
	}
	if !set {
		lines = append(lines[:start+1], append([]string{"  melody: " + name}, lines[start+1:]...)...)
	}
	return []byte(strings.Join(lines, "\n"))
}

// Switch shows the named app on the clock now.
func (s *Supervisor) Switch(name string) error {
	s.mu.Lock()
	inst := s.cur
	s.mu.Unlock()
	if inst == nil {
		return errors.New("bridge not running")
	}
	known := name == "Time" || name == "Date" // the clock's built-in apps
	for _, a := range inst.cfg.Apps {
		known = known || a.Name == name
	}
	topic := apps.SwitchTopic(inst.cfg.Awtrix.AppTopic)
	if !known || topic == "" {
		return fmt.Errorf("cannot switch to %q", name)
	}
	p, _ := json.Marshal(map[string]any{"name": name, "fast": true})
	return inst.pub.Publish(topic, p, false)
}

// SetArmed arms or disarms an app by publishing its retained arm state;
// the bridge's own subscription then applies it, so MQTT stays the single
// source of truth (and anything else, e.g. HA, can arm it the same way).
func (s *Supervisor) SetArmed(name string, on bool) error {
	s.mu.Lock()
	inst := s.cur
	s.mu.Unlock()
	if inst == nil {
		return errors.New("bridge not running")
	}
	topic, ok := inst.eng.ArmTopics()[name]
	if !ok {
		return fmt.Errorf("app %q does not use require_arm", name)
	}
	payload := []byte("off")
	if on {
		payload = []byte("on")
	}
	return inst.pub.Publish(topic, payload, true)
}

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

// ---- instance ----------------------------------------------------------

// instance is one run of the pipeline for one config.
type instance struct {
	cfg    *config.Config
	apps   []apps.App
	eng    *engine.Engine
	pub    *mqttPublisher
	cancel context.CancelFunc
	done   chan struct{}
	log    *slog.Logger

	activeMu sync.Mutex
	active   string // app the clock reports on screen
}

func (i *instance) activeApp() string {
	i.activeMu.Lock()
	defer i.activeMu.Unlock()
	return i.active
}

func start(parent context.Context, cfg *config.Config, prev []apps.App, log *slog.Logger) (*instance, error) {
	list, err := apps.Build(cfg)
	if err != nil {
		return nil, err
	}
	apps.Carry(prev, list)
	haClient, err := ha.New(cfg.HomeAssistant.URL, cfg.HomeAssistant.Token, log)
	if err != nil {
		return nil, fmt.Errorf("homeassistant.url: %w", err)
	}

	ctx, cancel := context.WithCancel(parent)
	inst := &instance{cfg: cfg, apps: list, pub: &mqttPublisher{}, cancel: cancel, done: make(chan struct{}), log: log}
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTT.Broker).
		SetClientID(cfg.MQTT.ClientID).
		SetUsername(cfg.MQTT.Username).
		SetPassword(cfg.MQTT.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(time.Minute).
		SetOnConnectHandler(func(c mqtt.Client) {
			log.Info("connected to mqtt", "broker", cfg.MQTT.Broker)
			// Retained arm states arrive right after subscribing, which
			// restores them after a restart.
			for name, topic := range inst.eng.ArmTopics() {
				c.Subscribe(topic, 1, func(_ mqtt.Client, m mqtt.Message) {
					inst.eng.SetArmed(name, string(m.Payload()) == "on")
				})
			}
			if t := cfg.Awtrix.ActiveTopic; t != "" {
				c.Subscribe(t, 0, func(_ mqtt.Client, m mqtt.Message) {
					inst.activeMu.Lock()
					inst.active = string(m.Payload())
					inst.activeMu.Unlock()
				})
			}
			inst.eng.ForceResync()
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Warn("mqtt connection lost", "err", err)
		})
	inst.pub.c = mqtt.NewClient(opts)
	inst.eng = engine.New(list, inst.pub, cfg.Awtrix.ResyncInterval.D(), log)
	inst.pub.c.Connect() // retries in the background (ConnectRetry)

	events := make(chan ha.Event, 64)
	go haClient.Run(ctx, events)
	go func() {
		defer close(inst.done)
		log.Info("awtrix-bridge started", "apps", len(list))
		inst.eng.Run(ctx, events)
		inst.pub.c.Disconnect(250)
	}()
	return inst, nil
}

func (i *instance) stop() {
	i.cancel()
	<-i.done
}

// removeDropped deletes apps from the clock that next no longer defines;
// pushed apps otherwise stay on the clock forever.
func (i *instance) removeDropped(next *config.Config) {
	keep := map[string]bool{}
	for _, a := range next.Apps {
		keep[a.Name] = true
	}
	for _, a := range i.cfg.Apps {
		if keep[a.Name] {
			continue
		}
		topic := strings.ReplaceAll(i.cfg.Awtrix.AppTopic, "{name}", a.Name)
		i.log.Info("removing app dropped from config", "app", a.Name)
		if err := i.pub.Publish(topic, nil, false); err != nil {
			i.log.Warn("remove app failed", "app", a.Name, "err", err)
		}
	}
}

type mqttPublisher struct{ c mqtt.Client }

func (p *mqttPublisher) Publish(topic string, payload []byte, retain bool) error {
	if !p.c.IsConnectionOpen() {
		return errors.New("mqtt not connected")
	}
	qos := byte(0)
	if retain {
		qos = 1
	}
	t := p.c.Publish(topic, qos, retain, payload)
	if !t.WaitTimeout(5 * time.Second) {
		return errors.New("publish timeout")
	}
	return t.Error()
}
