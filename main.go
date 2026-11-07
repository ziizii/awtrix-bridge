// awtrix-bridge pushes Home Assistant sensor values to an AWTRIX clock over MQTT.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/ziizii/awtrix-bridge/internal/apps"
	"github.com/ziizii/awtrix-bridge/internal/config"
	"github.com/ziizii/awtrix-bridge/internal/engine"
	"github.com/ziizii/awtrix-bridge/internal/ha"
)

func main() {
	cfgPath := flag.String("config", envOr("CONFIG", "/config/config.yaml"), "path to config file")
	flag.Parse()

	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	if err := run(*cfgPath, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfgPath string, log *slog.Logger) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	list, err := apps.Build(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pub := &mqttPublisher{}
	var eng *engine.Engine

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTT.Broker).
		SetClientID(cfg.MQTT.ClientID).
		SetUsername(cfg.MQTT.Username).
		SetPassword(cfg.MQTT.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(time.Minute).
		SetOnConnectHandler(func(mqtt.Client) {
			log.Info("connected to mqtt", "broker", cfg.MQTT.Broker)
			if eng != nil {
				eng.ForceResync()
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Warn("mqtt connection lost", "err", err)
		})
	pub.c = mqtt.NewClient(opts)
	eng = engine.New(list, pub, cfg.Awtrix.ResyncInterval.D(), log)
	pub.c.Connect() // retries in the background (ConnectRetry)
	defer pub.c.Disconnect(250)

	haClient, err := ha.New(cfg.HomeAssistant.URL, cfg.HomeAssistant.Token, log)
	if err != nil {
		return fmt.Errorf("homeassistant.url: %w", err)
	}
	events := make(chan ha.Event, 64)
	go haClient.Run(ctx, events)

	log.Info("awtrix-bridge started", "apps", len(list))
	eng.Run(ctx, events)
	log.Info("shutting down")
	return nil
}

type mqttPublisher struct{ c mqtt.Client }

func (p *mqttPublisher) Publish(topic string, payload []byte) error {
	if !p.c.IsConnectionOpen() {
		return fmt.Errorf("mqtt not connected")
	}
	t := p.c.Publish(topic, 0, false, payload)
	if !t.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("publish timeout")
	}
	return t.Error()
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
