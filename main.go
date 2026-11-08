// awtrix-bridge pushes Home Assistant sensor values to an AWTRIX clock over MQTT.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ziizii/awtrix-bridge/internal/admin"
	"github.com/ziizii/awtrix-bridge/internal/bridge"
)

func main() {
	cfgPath := flag.String("config", envOr("CONFIG", "/config/config.yaml"), "path to config file")
	adminAddr := flag.String("admin", os.Getenv("ADMIN_ADDR"), "admin UI listen address, e.g. :8080 (empty = off)")
	flag.Parse()

	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	sup := bridge.New(*cfgPath, log)
	if *adminAddr != "" {
		srv := &http.Server{Addr: *adminAddr, Handler: admin.Handler(sup), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			log.Info("admin UI listening", "addr", *adminAddr)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("admin UI", "err", err)
			}
		}()
		defer srv.Close()
	}

	if err := sup.Run(ctx); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
