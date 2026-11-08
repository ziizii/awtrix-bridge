// The bridge itself: connection and uptime.

import { el, card, ago, pretty, dateTime } from "../dom.js";

let values;

const rows = [
  ["mqtt", "MQTT broker"],
  ["active", "On screen"],
  ["started", "Running since"],
  ["loaded", "Config loaded"],
];

export function mount() {
  const c = card("Bridge");
  const dl = el("dl", "kv");
  values = {};
  for (const [key, label] of rows) {
    values[key] = el("dd");
    dl.append(el("dt", null, label), values[key]);
  }
  c.root.append(dl);
  return c.root;
}

export function render(s) {
  values.mqtt.textContent = s.mqtt ? "Connected" : "Disconnected";
  values.mqtt.style.color = s.mqtt ? "var(--ok)" : "var(--danger)";
  values.active.textContent = s.active ? pretty(s.active) : "Unknown";
  values.started.textContent = ago(s.started);
  values.started.title = dateTime(s.started);
  values.loaded.textContent = ago(s.loaded);
  values.loaded.title = dateTime(s.loaded);
}
