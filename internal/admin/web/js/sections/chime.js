// Hourly chime: pick a melody (saved to the config), preview it on the
// clock, and see the quiet time that silences it.

import { el, card, keyed, hhmm } from "../dom.js";
import { api } from "../api.js";
import { icon } from "../icons.js";
import { melodyChoices, playable, OFF, CUSTOM } from "../melodies.js";

let root, next, quiet, opts;

export function mount() {
  const c = card("Hourly chime", "Plays on the clock at every full hour");
  root = c.root;
  next = el("span", "chip");
  c.head.append(next);

  quiet = el("div", "banner");
  quiet.parts = { ic: el("div", "banner-ic"), title: el("div", "banner-title"), sub: el("div", "banner-sub"), pill: el("span", "chip") };
  const text = el("div", "banner-text");
  text.append(quiet.parts.title, quiet.parts.sub);
  quiet.append(quiet.parts.ic, text, quiet.parts.pill);

  opts = el("div", "opts");
  opts.setAttribute("role", "radiogroup");
  opts.setAttribute("aria-label", "Chime melody");
  root.append(quiet, opts);
  root.hidden = true;
  return root;
}

export function render(s, ui) {
  const melodies = s.melodies || [];
  root.hidden = melodies.length === 0;
  const ch = s.chime || {};
  renderNext(ch);
  renderQuiet(ch.quiet);
  keyed(opts, melodyChoices(melodies, "Off", ch.melody), m => m.name, m => createOpt(m, ui), (row, m) => {
    const sel = m.name === ch.melody;
    row.classList.toggle("sel", sel);
    row.setAttribute("aria-checked", sel);
    if (sel) row.classList.remove("pending");
  });
}

function renderNext(ch) {
  if (ch.melody === OFF) {
    next.textContent = "Off";
    next.className = "chip";
    return;
  }
  const at = new Date();
  at.setMinutes(60, 0, 0); // the next full hour
  const q = ch.quiet;
  const silent = q && q.from && q.to && at >= new Date(q.from) && at < new Date(q.to);
  next.textContent = "Next " + hhmm(at) + (silent ? " · silent" : "");
  next.className = "chip " + (silent ? "night" : "accent");
}

function renderQuiet(q) {
  const p = quiet.parts;
  quiet.classList.toggle("night", !!q);
  quiet.classList.toggle("on", !!(q && q.on));
  if (!q) {
    p.ic.replaceChildren(icon("sun"));
    p.title.textContent = "No quiet time";
    p.sub.textContent = "Chimes day and night. Set quiet_entity to silence it.";
    p.pill.hidden = true;
    return;
  }
  p.ic.replaceChildren(icon("moon"));
  p.title.textContent = q.from && q.to ? `Quiet ${hhmm(q.from)}–${hhmm(q.to)}` : "Quiet time";
  p.sub.textContent = q.name || q.entity;
  p.sub.title = q.entity;
  p.pill.hidden = false;
  p.pill.textContent = q.on ? "Quiet now" : q.state === "off" ? "Not now" : q.state;
  p.pill.className = "chip" + (q.on ? " night" : "");
}

// A melody label reads "Tick: one short blip": name and description.
function split(label) {
  const i = label.indexOf(":");
  return i < 0 ? [label, ""] : [label.slice(0, i), label.slice(i + 1).trim()];
}

function createOpt(m, ui) {
  const row = el("div", "opt");
  row.setAttribute("role", "radio");
  row.tabIndex = 0;
  let [name, desc] = split(m.label);
  if (m.name === OFF) desc = "No chime";
  if (m.name === CUSTOM) [name, desc] = ["Custom", "A tune set in the config file"];
  const text = el("div", "opt-text");
  text.append(el("div", "opt-name", name), el("div", "opt-desc", desc));
  row.append(el("span", "radio"), text);

  if (playable(m.name)) {
    const play = el("button", "icon-btn");
    play.type = "button";
    play.title = "Preview on the clock";
    play.setAttribute("aria-label", "Play " + name);
    play.append(icon("play"));
    play.onclick = e => {
      e.stopPropagation();
      api.playMelody(m.name).catch(err => ui.toast(err.message, "err"));
    };
    row.append(play);
  }

  if (m.name === CUSTOM) row.classList.add("locked"); // can only be set in the file
  const choose = async () => {
    if (m.name === CUSTOM || row.classList.contains("sel") || row.classList.contains("pending")) return;
    row.classList.add("pending");
    try {
      await api.setChime(m.name);
      ui.configChanged();
    } catch (e) {
      ui.toast(e.message, "err");
    }
    setTimeout(() => row.classList.remove("pending"), 3000);
  };
  row.onclick = choose;
  row.onkeydown = e => {
    if (e.key === "Enter" || e.key === " ") { e.preventDefault(); choose(); }
  };
  return row;
}
