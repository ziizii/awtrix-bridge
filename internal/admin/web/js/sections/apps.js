// Every app on the clock with a live preview. Tapping one switches the
// clock to it; the clock's built-in Time and Date come first.

import { el, card, keyed, ago, pretty } from "../dom.js";
import { api } from "../api.js";
import { Matrix } from "../matrix.js";
import { clockApps } from "../frames.js";

let grid;

export function mount() {
  const c = card("Apps", "Tap one to show it on the clock");
  grid = el("div", "tiles");
  c.root.append(grid);
  return c.root;
}

export function render(s, ui) {
  // The timer has its own card; list it only while it runs.
  const refused = new Map((s.rejected || []).map(r => [r.topic, r.reason]));
  const items = clockApps(s).filter(p => !p.frame.setting && !(p.app === "timer" && p.frame.off));
  keyed(grid, items, p => p.app, () => createTile(ui), (tile, p) => {
    const t = tile.parts;
    const active = p.app === s.active;
    t.item = p;
    t.matrix.show(p.frame);
    t.name.textContent = pretty(p.app);
    t.dot.hidden = !active;
    const reason = refused.get(p.topic);
    t.sub.textContent = reason ? "Rejected by the clock" : active ? "On screen" : p.frame.off ? "Hidden" : p.builtin ? "Built in" : "Updated " + ago(p.at);
    tile.classList.toggle("refused", !!reason);
    tile.disabled = !(p.builtin || (p.pushed && !p.frame.off));
    tile.title = p.builtin ? "Built into the clock" : p.topic + (reason ? "\nRejected: " + reason : "");
    tile.classList.toggle("active", active);
    tile.classList.toggle("off", !!p.frame.off);
    if (active) tile.classList.remove("pending");
  });
}

function createTile(ui) {
  const tile = el("button", "tile");
  tile.type = "button";
  const t = tile.parts = { matrix: new Matrix(), name: el("span"), dot: el("span", "live-dot"), sub: el("div", "tile-sub"), item: null };
  const screen = el("div", "screen");
  screen.append(t.matrix.el);
  const nameRow = el("div", "tile-name");
  nameRow.append(t.name, t.dot);
  tile.append(screen, nameRow, t.sub);
  tile.onclick = async () => {
    if (tile.classList.contains("active")) return;
    tile.classList.add("pending");
    setTimeout(() => tile.classList.remove("pending"), 4000);
    try {
      await api.switchTo(t.item.app);
      ui.refresh(400);
    } catch (e) {
      tile.classList.remove("pending");
      ui.toast(e.message, "err");
    }
  };
  return tile;
}
