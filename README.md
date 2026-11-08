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
| `countdown` | `m:ss` to a timestamp sensor, then counts up (`+0:42`) for `hold_after`. Shown within `show_within` (`0s` = always). Plays `alert_melody` (or a custom `alert_rtttl`) at `alert_at`, once per bus, or once per arming with `require_arm`. Switches the clock to it once at `switch_at` (`0` = never) and back to `switch_back` (e.g. `Time`) when the count-up ends. With `require_arm` it is hidden until armed and disarms itself after the next arrival. |

Top-level `brightness:` switches the clock to a quiet brightness while an
entity is on, and `chime:` plays a short melody at every full hour unless
its `quiet_entity` is on. Pick the melody (or off) in the admin UI, which
previews each one on the clock; `melody:` names a built-in tune, or leave it
empty and set a custom `rtttl:`. The chime plays right on the hour; with a time-of-day
`quiet_entity` (HA `tod`) its `after`/`before` window decides, since the
entity itself flips on the hour.

### Timer

A built-in app named `timer`, started from the admin UI for a duration or
until a time of day. It switches the clock to itself, counts down with a
progress bar, plays its melody at the end, counts up in red for
`hold_after` (default `1m`), then switches back to the app that was on
screen when it started. Its state is retained on `timer_topic`
(`awtrix-bridge/timer`, JSON `{"start": ..., "end": ..., "back": ...}`,
empty = none), so a running timer survives restarts and anything that can
publish MQTT can start or cancel one. Options go under an optional
top-level `timer:` (see `config.example.yaml`); no app may be named `timer`.

Icons are LaMetric icon numbers. The clock only draws icons stored on it,
so after every start the bridge uploads the ones the config uses that the
clock lacks (it finds the clock's address on `device_topic`,
`awtrix/state/device`), fetched from LaMetric. The timer's icon is picked
in the admin UI.

Apps whose entity is `unavailable`/`unknown` are removed from the clock
(empty payload) until the value comes back. Any extra AWTRIX field
(`duration`, `rainbow`, `pushIcon`, …) can be added under `extra:`.

### Arming (`require_arm`)

For a countdown you only need sometimes (e.g. picking someone up from the
bus). The arm state is a retained MQTT message on `arm_topic`
(`awtrix-bridge/<app>/armed`, payload `on`/`off`), so it survives restarts.
Arm it with the button in the admin UI, or from anything that can publish
MQTT, e.g. an HA script:

```yaml
action: mqtt.publish
data: {topic: awtrix-bridge/bus333/armed, payload: "on", retain: true}
```

Only a bus arriving after the moment of arming disarms it.

## Admin UI

With `ADMIN_ADDR` set (compose: `:8080`, published on host port `8095`) the
bridge serves a small page, built for phones first:

- **Now**: the clock's screen as an LED preview; pickup (arm/disarm
  `require_arm` countdowns, set their alert); the timer (presets, custom
  minutes or until a time; add time or cancel while it runs; its melody);
  and every app with a preview; tap one (including the built-in Time and
  Date) to switch the clock to it.
- **Sounds**: the hourly chime melody or off, with previews and the quiet time.
- **Look**: colors and icons. The clock's own colors (time, date, default
  text, weekday bar; sent to the clock, which keeps them), the timer's
  (text, time's up, progress bar and track) and each app's (single color,
  surplus/deficit, or each threshold), plus the timer's and each app's
  icon. The color picker stays open while you try colors: the clock
  switches to what you are coloring and shows each one live (nothing is
  saved) until **Done** saves it or **Cancel** restores it. Timer and app
  settings are written to `config.yaml`.
- **Activity**: recent switches, sounds and arming, and the bridge's status.
- **Config**: a YAML editor. **Save** validates the config,
  writes `config.yaml` and restarts the pipeline in place; apps removed from
  the config are deleted from the clock.

Settings changed with the pickers are written into `config.yaml` as line
edits, so comments and layout stay as they are. There is no
password, so keep it on the LAN. `${VAR}` references stay unexpanded in the
editor, so secrets in `.env` are never shown.

The container must be able to write `config.yaml`: compose runs it as
`PUID:PGID` (default `1000:1000`), the owner of the file.

## Run

Deployed only on the `containers` host, built from this repo (no registry,
no CI). See [AGENTS.md](AGENTS.md) for the deploy steps. First-time setup:

```bash
git clone git@github.com:ziizii/awtrix-bridge.git ~/repos/awtrix-bridge
cd ~/repos/awtrix-bridge
cp config.example.yaml config.yaml && cp .env.example .env   # fill in HA_TOKEN
docker compose up -d --build
docker compose logs -f
```

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
go run . -config config.yaml -admin :8095
```
