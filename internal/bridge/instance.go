package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/awtrix"
	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/engine"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

// instance is one run of the pipeline for one config: an MQTT client, an
// HA client and the engine between them.
type instance struct {
	cfg    *config.Config
	topics awtrix.Topics
	apps   []apps.App
	eng    *engine.Engine
	mqtt   mqtt.Client
	log    *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}

	activeMu sync.Mutex
	active   string // app the clock reports on screen
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
	i := &instance{
		cfg: cfg, topics: awtrix.NewTopics(cfg.Awtrix), apps: list, log: log,
		cancel: cancel, done: make(chan struct{}),
	}
	i.mqtt = mqtt.NewClient(i.mqttOptions())
	i.eng = engine.New(list, i, cfg.Awtrix.ResyncInterval.D(), log)
	i.mqtt.Connect() // retries in the background (ConnectRetry)

	events := make(chan ha.Event, 64)
	go haClient.Run(ctx, events)
	go func() {
		defer close(i.done)
		log.Info("awtrix-bridge started", "apps", len(list))
		i.eng.Run(ctx, events)
		i.mqtt.Disconnect(250)
	}()
	return i, nil
}

func (i *instance) stop() {
	i.cancel()
	<-i.done
}

func (i *instance) mqttOptions() *mqtt.ClientOptions {
	c := i.cfg.MQTT
	return mqtt.NewClientOptions().
		AddBroker(c.Broker).
		SetClientID(c.ClientID).
		SetUsername(c.Username).
		SetPassword(c.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(time.Minute).
		SetOnConnectHandler(i.onConnect).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			i.log.Warn("mqtt connection lost", "err", err)
		})
}

// onConnect (re)subscribes and republishes everything; it runs after
// every reconnect, since the session is not persistent.
func (i *instance) onConnect(c mqtt.Client) {
	i.log.Info("connected to mqtt", "broker", i.cfg.MQTT.Broker)
	// Retained arm states arrive right after subscribing, which restores
	// them after a restart.
	for name, topic := range i.eng.ArmTopics() {
		c.Subscribe(topic, 1, func(_ mqtt.Client, m mqtt.Message) {
			i.eng.SetArmed(name, awtrix.IsArmed(m.Payload()))
		})
	}
	c.Subscribe(i.topics.Active, 0, func(_ mqtt.Client, m mqtt.Message) {
		i.activeMu.Lock()
		i.active = string(m.Payload())
		i.activeMu.Unlock()
	})
	i.eng.ForceResync()
}

func (i *instance) activeApp() string {
	i.activeMu.Lock()
	defer i.activeMu.Unlock()
	return i.active
}

// Publish implements engine.Publisher.
func (i *instance) Publish(topic string, payload []byte, retain bool) error {
	if !i.mqtt.IsConnectionOpen() {
		return errors.New("mqtt not connected")
	}
	qos := byte(0)
	if retain {
		qos = 1
	}
	t := i.mqtt.Publish(topic, qos, retain, payload)
	if !t.WaitTimeout(5 * time.Second) {
		return errors.New("publish timeout")
	}
	return t.Error()
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
		i.log.Info("removing app dropped from config", "app", a.Name)
		if err := i.Publish(i.topics.App(a.Name), nil, false); err != nil {
			i.log.Warn("remove app failed", "app", a.Name, "err", err)
		}
	}
}
