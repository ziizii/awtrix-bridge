// On the clock: what every app currently shows. Tapping one switches the
// clock to it; the clock's built-in Time and Date are listed first.

import { $, el, keyed, ago, hhmm } from "../dom.js";
import { api } from "../api.js";
import { led, setLed } from "../led.js";

export function render(s, ui) {
  const items = builtins().concat((s.apps || []).map(p => ({
    ...p,
    switchable: p.topic.includes("/pushed/") && !!p.payload, // a visible pushed app
  })));
  keyed($("apps"), items, p => p.app, () => createTile(ui), (tile, p) => {
    tile.parts.item = p;
    setLed(tile.parts.led, p.payload);
    tile.parts.label.textContent = p.app;
    tile.disabled = !p.switchable;
    tile.classList.toggle("active", p.app === s.active);
    tile.title = p.at ? p.topic + "\nupdated " + ago(p.at) : "clock built-in app";
  });
}

// The clock draws these itself; the preview just mirrors the browser clock.
function builtins() {
  const now = new Date();
  const text = t => JSON.stringify({ text: t });
  return [
    { app: "Time", payload: text(hhmm(now)), switchable: true },
    { app: "Date", payload: text(now.toLocaleDateString([], { day: "numeric", month: "numeric" })), switchable: true },
  ];
}

function createTile(ui) {
  const tile = el("button", "app");
  tile.parts = { label: el("div", "n"), led: led(), item: null };
  tile.append(tile.parts.label, tile.parts.led);
  tile.onclick = async () => {
    try {
      await api.switchTo(tile.parts.item.app);
      ui.refresh(400);
    } catch (e) { alert(e.message); }
  };
  return tile;
}
