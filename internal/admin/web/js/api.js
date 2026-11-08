// The bridge's JSON API (see internal/admin/admin.go).

async function call(method, path, body) {
  const opt = { method, headers: {} };
  if (body !== undefined) {
    opt.headers["Content-Type"] = "application/json";
    opt.body = JSON.stringify(body);
  }
  const r = await fetch(path, opt);
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || r.statusText);
  return j;
}

export const api = {
  status: () => call("GET", "/api/status"),
  config: () => call("GET", "/api/config").then(j => j.yaml),
  check: yaml => call("POST", "/api/check", { yaml }),
  saveConfig: yaml => call("PUT", "/api/config", { yaml }),
  reload: () => call("POST", "/api/reload", {}),
  switchTo: app => call("POST", "/api/switch", { app }),
  arm: (app, armed) => call("POST", "/api/arm", { app, armed }),
  playMelody: melody => call("POST", "/api/melody/play", { melody }),
  setChime: melody => call("POST", "/api/chime", { melody }),
  setAlert: (app, minutes, melody) => call("POST", "/api/alert", { app, minutes, melody }),
  startTimer: seconds => call("POST", "/api/timer", { seconds }),
  startTimerAt: at => call("POST", "/api/timer", { at }),
  extendTimer: seconds => call("POST", "/api/timer/extend", { seconds }),
  cancelTimer: () => call("POST", "/api/timer/cancel", {}),
  setColor: (target, app, key, value) => call("POST", "/api/color", { target, app, key, value }),
  previewColor: (target, app, key, value) => call("POST", "/api/color/preview", { target, app, key, value }),
  endPreview: (target, app) => call("POST", "/api/color/end", { target, app }),
  setAppIcon: (app, key, icon) => call("POST", "/api/icon", { app, key, icon }),
  setTimerIcon: icon => call("POST", "/api/timer/icon", { icon }),
  setTimerMelody: melody => call("POST", "/api/timer/melody", { melody }),
};
