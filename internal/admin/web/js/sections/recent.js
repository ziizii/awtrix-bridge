// Recent switches, sounds and arm changes, newest first.

import { el, card, hhmmss, pretty } from "../dom.js";
import { icon } from "../icons.js";

let list, empty, sig = "";

export function mount() {
  const c = card("Recent", "Switches, sounds and arming");
  list = el("ul", "events");
  empty = el("div", "empty-state");
  empty.append(icon("activity"), el("div", null, "Nothing yet. Events show up here as they happen."));
  c.root.append(list, empty);
  return c.root;
}

export function render(s) {
  const events = (s.recent || []).slice().reverse();
  const next = JSON.stringify(events);
  if (next === sig) return;
  sig = next;
  empty.hidden = events.length > 0;
  list.replaceChildren(...events.map(p => {
    const d = describe(p);
    const li = el("li", "event " + d.kind);
    const ic = el("div", "ev-ic");
    ic.append(icon(d.icon));
    const main = el("div", "ev-main");
    main.append(el("div", "ev-title", d.title), el("div", "ev-detail", d.detail));
    const time = el("time", null, hhmmss(p.at));
    time.dateTime = p.at;
    li.append(ic, main, time);
    li.title = p.topic + "\n" + p.payload;
    return li;
  }));
}

// describe reads an event from its payload (see internal/awtrix).
function describe(p) {
  const from = "from " + pretty(p.app);
  if (p.payload === "on" || p.payload === "off") {
    return { kind: "arm", icon: "bus", title: (p.payload === "on" ? "Armed " : "Disarmed ") + pretty(p.app), detail: p.topic };
  }
  let j = {};
  try { j = JSON.parse(p.payload) || {}; } catch { /* not JSON */ }
  if (j.name) return { kind: "switch", icon: "swap", title: "Switched to " + pretty(j.name), detail: from };
  if (j.rtttl) {
    const tune = String(j.rtttl).split(":")[0];
    return { kind: "sound", icon: "music", title: "Played " + (tune ? pretty(tune) : "a tune"), detail: from };
  }
  return { kind: "other", icon: "activity", title: pretty(p.app), detail: p.topic + " " + p.payload };
}
