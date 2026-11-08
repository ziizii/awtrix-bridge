// The connection pill in the top bar.

import { $, ago } from "../dom.js";

function set(cls, text, title) {
  const c = $("conn");
  c.className = "conn " + cls;
  c.querySelector(".conn-text").textContent = text;
  c.title = title;
}

export function render(s) {
  document.body.classList.remove("stale");
  if (s.mqtt) set("ok", "Live", "Connected to MQTT · config loaded " + ago(s.loaded));
  else set("bad", "MQTT down", "The bridge is not connected to the MQTT broker");
}

export function unreachable(err) {
  document.body.classList.add("stale");
  set("bad", "Offline", "Bridge unreachable: " + err.message);
}
