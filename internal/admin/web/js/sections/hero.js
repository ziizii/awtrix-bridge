// The clock's screen as it is now: the app on screen, drawn LED by LED.

import { el, ago, pretty } from "../dom.js";
import { icon } from "../icons.js";
import { Matrix } from "../matrix.js";
import { clockApps, brightness } from "../frames.js";

let matrix, name, sub, light;

export function mount() {
  const root = el("div", "card hero");
  matrix = new Matrix({ glow: true });
  const device = el("div", "device");
  device.append(matrix.el);

  const meta = el("div", "hero-meta");
  const text = el("div");
  name = el("div", "hero-name", "—");
  sub = el("div", "hero-sub");
  text.append(el("div", "eyebrow", "On screen"), name, sub);
  light = el("span", "chip");
  light.append(icon("sun"), el("span"));
  light.hidden = true;
  const chips = el("div", "chips");
  chips.append(light);
  meta.append(text, chips);

  root.append(device, meta);
  return root;
}

export function render(s) {
  const cur = clockApps(s).find(p => p.app === s.active);
  matrix.show(cur ? cur.frame : { off: true });
  name.textContent = s.active ? pretty(s.active) : "Unknown";
  sub.textContent = !s.active ? "Waiting for the clock to report its app"
    : !cur ? "Not an app the bridge knows"
    : cur.builtin ? "Built into the clock"
    : "Updated " + ago(cur.at);

  const b = brightness(s);
  light.hidden = b == null;
  if (b != null) {
    light.lastChild.textContent = Math.round((b / 255) * 100) + "%";
    light.title = "Brightness " + b + " of 255";
  }
}
