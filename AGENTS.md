# Agent notes

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
- Clock: awtrixng firmware at 192.168.11.199. `autoTransition` is off, so it
  stays on whatever app it is switched to. An app must be enabled in the
  clock's own loop (`/apploop.json`, `PUT /api/v1/apps/order`) or switching to
  it does nothing. The active app is reported on `awtrix/state/apps/active`.
- Home Assistant: 192.168.11.187:8123.
