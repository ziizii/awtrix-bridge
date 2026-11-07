# awtrix-bridge

Small Go service that pushes Home Assistant values to an AWTRIX clock over MQTT.

- Subscribes to HA's WebSocket API (`state_changed`) — no polling, no HA automations.
- Renders apps from `config.yaml` and publishes directly to the MQTT broker.
- Only publishes when a payload actually changes; re-pushes everything every
  `resync_interval` and after an MQTT reconnect so the clock recovers after a reboot.
- **Countdown apps tick locally every second** (e.g. next bus arrival). HA only
  sends the target timestamp; the per-second updates never touch HA.

## App types

| type        | what it shows                                                          |
|-------------|------------------------------------------------------------------------|
| `value`     | one numeric sensor, `format` (Go fmt), fixed `color` or `colors` thresholds |
| `surplus`   | `production - consumption` as `+1.2kW` / `-850W`, night icon when PV = 0 |
| `countdown` | `m:ss` to a timestamp sensor, shown within `show_within`, optional beep at `alert_at` and auto-switch |

Apps whose entity is `unavailable`/`unknown` are removed from the clock
(empty payload) until the value comes back. Any extra AWTRIX field
(`duration`, `rainbow`, `pushIcon`, …) can be added under `extra:`.

## Run

Every push to `main` builds `ghcr.io/ziizii/awtrix-bridge:latest` (amd64 + arm64)
via GitHub Actions. On the server you only need three files:

```bash
mkdir -p ~/awtrix-bridge && cd ~/awtrix-bridge
# copy docker-compose.yml, config.example.yaml -> config.yaml, .env.example -> .env
docker login ghcr.io -u ziizii          # PAT with read:packages (repo is private)
docker compose pull && docker compose up -d
docker compose logs -f
```

Update later with `docker compose pull && docker compose up -d`.
To build from source instead, clone the repo and use `build: .` in the compose file.

Set `LOG_LEVEL=debug` to log every MQTT publish.

### Credentials

- **HA token**: HA → Profile → Security → Long-lived access tokens.
- **MQTT**: use the broker HA's MQTT integration points at (Settings → Devices
  & services → MQTT). Here that is the TeslaMate Mosquitto on the containers
  host (`192.168.11.239:1883`), which allows anonymous clients, so username
  and password stay empty. The integration is titled "Mosquitto broker", but
  the HA Mosquitto app is not the broker the clock uses.

### Clock firmware notes

The clock runs `awtrixng` firmware. An empty payload on
`awtrix/cmd/apps/pushed/<app>` deletes the app (verified). Apps can also be
disabled in the clock's own app loop (`http://<clock>/apploop.json`,
`/api/v1/apps`); a disabled app receives payloads but is never shown.

## Migrating from the HA automations

Once the bridge is running, disable `automation.awtrix_update_apps`,
`automation.bus_arrival_notification` and `automation.bus_countdown_reset`,
otherwise both will publish to the same topics.

## Development

```bash
go test ./...
go run . -config config.yaml
```
