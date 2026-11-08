// Connection state in the page header.

import { $, el, ago } from "../dom.js";

export function render(s) {
  $("meta").replaceChildren(
    el("span", "dot" + (s.mqtt ? " on" : "")),
    (s.mqtt ? "MQTT connected" : "MQTT disconnected") + " · config loaded " + ago(s.loaded),
  );
}

export function unreachable(err) {
  $("meta").textContent = "bridge unreachable: " + err.message;
}
