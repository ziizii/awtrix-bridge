# Agent notes

## Working here

This is a small hobby project driving the clock on the owner's desk.
Commit, push and deploy finished work without asking first; just report
what was deployed. Times in the UI and on the clock are 24-hour (no
AM/PM), including inputs: don't rely on `<input type="time">`, which
follows the browser locale.

## Deploy

awtrix-bridge runs only on the `containers` host (192.168.11.239), built
from the clone at `~/repos/awtrix-bridge`. There is no registry and no CI.
Deploying means: push to `main`, then pull and rebuild on containers.

```bash
git push
ssh -4 containers 'cd ~/repos/awtrix-bridge && git pull --ff-only && docker compose up -d --build'
ssh -4 containers 'cd ~/repos/awtrix-bridge && docker compose logs --since 1m'
```

- Always `ssh -4`: IPv6 to containers fails with "No route to host".
- `config.yaml` and `.env` on the server are gitignored and hold the live
  config; never overwrite them blindly. The admin UI edits `config.yaml`
  in place, so check it before replacing it.
- The admin UI is https://clock-admin.zverina.biz (local nginx on containers,
  host port 8095). After a deploy, the first request can return 502 while
  the container restarts.

## Architecture

Data flows HA WebSocket -> engine -> apps -> MQTT -> clock. Packages under
`internal/`:

| package  | role |
|----------|------|
| `config` | YAML types, defaults, validation (`validate.go`), built-in melodies (`melody.go`), and line-based edits that keep the file's comments (`edit.go`). |
| `awtrix` | The clock's MQTT protocol: topics and payload builders. The only place that builds clock JSON. |
| `apps`   | One file per app type (`value`, `surplus`, `countdown`, `brightness`, `chime`). `app.go` has the `App` interface, `Build` and `Carry`. |
| `engine` | Single goroutine that owns all app state: routes HA states, ticks every second, dedupes and publishes. `status.go` mirrors state for other goroutines. |
| `ha`     | Minimal HA WebSocket client (state snapshot + `state_changed`). |
| `bridge` | `Supervisor` owns the config file and the running `instance` (MQTT + HA + engine) and swaps it on reload. `actions.go` = what the UI can do, `status.go` = what it shows. |
| `admin`  | HTTP API (`admin.go`, one generic `action` handler) and the page in `web/` (plain ES modules: views in `web/js/views.js`, one module per section in `web/js/sections/`, the LED preview in `matrix.js`/`frames.js`). |

Common changes:

- **New app type**: add a `config.Type*` constant and its fields plus
  validation, a file in `apps/` implementing `App` (and `carry` if it keeps
  state across reloads), and a case in `apps.Build`. Build clock payloads with
  `awtrix`. Apps run on the engine goroutine only, so plain fields are fine.
- **New admin action**: a method on `bridge.Supervisor` (an immediate MQTT
  effect, or a config edit via `s.edit` + `config.SetApp`/`SetChime`), the
  method in `admin.Backend`, one route line in `admin.Handler`, a call in
  `web/js/api.js`, and the UI in a section module.
- **New admin UI section or page**: a module in `web/js/sections/` exporting
  `mount(ui)` (returns its element, built once) and `render(status, ui)`,
  listed in a view in `web/js/views.js`; a new view gets its own tab. Style
  it with the tokens and components in `style.css` (`card`, `btn`, `chip`,
  `field`), which have light and dark values.
- **Changing app state from outside** (MQTT handler, UI): go through
  `engine.exec`, never touch apps from another goroutine.
- **Admin UI sections** re-render every second: build elements once and update
  them in place with `keyed()` (rebuilding swallows clicks), and keep element
  references in `node.parts`, not as properties on the element.

## Test

There is no Go toolchain on the Mac. Run tests on containers (`/usr/bin/go`).
To test uncommitted work, copy the tree over first:

```bash
rsync -a -e "ssh -4" --exclude .git --exclude .env --exclude config.yaml ./ containers:repos/awtrix-bridge/
ssh -4 containers 'cd ~/repos/awtrix-bridge && gofmt -l . ; go vet ./... && go test -race ./...'
```

Afterwards, reset the server clone with `git checkout -- . && git clean -fd`
before `git pull` (both keep the ignored `config.yaml` and `.env`).

## Commits

Use the GitHub noreply email `ziizii@users.noreply.github.com`; GitHub rejects
pushes containing the private address. The local repo config already sets it.

## Environment facts

- MQTT broker: TeslaMate mosquitto on containers (`192.168.11.239:1883`,
  anonymous). `mosquitto_sub`/`mosquitto_pub` via
  `docker exec mac-main-teslamate-mosquitto-1 ...`.
- Clock: awtrixng firmware at 192.168.11.199. It reports its settings (time
  format, colors, scrolling) retained on `awtrix/state/settings`; the bridge
  passes them to the UI as `status.clock` for the LED preview.
- The firmware is strict: it rejects a whole payload with any key it does
  not know (e.g. it has `progress`/`progressColor`/`progressTrackColor`,
  not AWTRIX 3's `progressC`/`progressBC`; the track is white unless set) and answers every command on `<topic>/result`
  (`{"ok":false,"error":{...}}`). The bridge logs rejections and the UI
  lists them under Activity; probe a new key with `mosquitto_pub` to a
  scratch app before relying on it. `GET /api/v1/display/screen` on the
  clock returns its 32×8 pixels, to check what it actually draws. `autoTransition` is off, so it
  stays on whatever app it is switched to. An app must be enabled in the
  clock's own loop (`/apploop.json`, `PUT /api/v1/apps/order`) or switching to
  it does nothing. The active app is reported on `awtrix/state/apps/active`.
- Home Assistant: 192.168.11.187:8123.
