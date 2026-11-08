// Colors: the clock's own (time, date, weekday bar; saved on the clock),
// the timer's and each app's (saved in config.yaml). Each card shows a
// preview that follows the picker while it is dragged; a color is saved
// when the picker closes. The status lists what can be changed
// (status.colors, see internal/bridge/colors.go).

import { el, card, keyed, pretty } from "../dom.js";
import { api } from "../api.js";
import { icon } from "../icons.js";
import { Matrix } from "../matrix.js";
import { clockApps } from "../frames.js";

let ui;
let clockPart, timerPart, appList;
// Colors picked here but not yet back in the status: "target|app|key" -> value.
const local = new Map();
const id = (target, app, key) => `${target}|${app}|${key}`;

export function mount(u) {
  ui = u;
  const root = el("div", "stack");

  const clock = card("Clock", "Its built-in Time and Date. Saved on the clock.");
  clockPart = { matrix: new Matrix(), rows: el("div", "color-rows") };
  clock.root.append(screen(clockPart.matrix), clockPart.rows);

  const timer = card("Timer", "While it runs, and when time is up.");
  timerPart = { run: new Matrix(), done: new Matrix(), rows: el("div", "color-rows") };
  const pair = el("div", "screen-pair");
  pair.append(screen(timerPart.run), screen(timerPart.done));
  timer.root.append(pair, timerPart.rows);

  const apps = card("Apps", "Each app's text. Thresholds pick a color by value.");
  appList = el("div", "app-colors-list");
  apps.root.append(appList);

  root.append(clock.root, timer.root, apps.root);
  return root;
}

function screen(m) {
  const s = el("div", "screen");
  s.append(m.el);
  return s;
}

export function render(s) {
  const colors = s.colors;
  if (!colors) return;
  const textColor = (s.clock && s.clock.textColor) || "#FFFFFF";

  // Clock: the Time app drawn with the clock's settings plus local edits.
  rows(clockPart.rows, colors.clock, "clock", "", textColor);
  clockPart.matrix.show(clockApps({ clock: withEdits(s.clock || {}, colors.clock) }).find(a => a.app === "Time").frame);

  // Timer: a sample while running and after the end.
  rows(timerPart.rows, colors.timer, "timer", "", textColor);
  const t = Object.fromEntries(colors.timer.map(f => [f.key, value("timer", "", f) || textColor]));
  timerPart.run.show({ text: "4:05", color: t.color, progress: 40, progressColor: t.progress_color, progressTrack: t.track_color });
  timerPart.done.show({ text: "+0:12", color: t.done_color });

  // Apps: the live payload, recolored while a picker is dragged.
  const live = new Map(clockApps(s).map(a => [a.app, a]));
  keyed(appList, colors.apps, a => a.app, a => {
    const box = el("div", "app-colors");
    box.parts = { matrix: new Matrix(), rows: el("div", "color-rows"), name: el("div", "app-colors-name", pretty(a.app)) };
    const head = el("div", "app-colors-head");
    head.append(screen(box.parts.matrix), box.parts.name);
    box.append(head, box.parts.rows);
    return box;
  }, (box, a) => {
    rows(box.parts.rows, a.fields, "app", a.app, textColor);
    const shown = live.get(a.app);
    let frame = shown && !shown.frame.off ? { ...shown.frame } : { text: pretty(a.app), icon: shown && shown.frame.icon };
    if (!shown || shown.frame.off) frame.color = value("app", a.app, a.fields[a.fields.length - 1]) || textColor;
    if (box.parts.editing) frame = { ...frame, color: box.parts.editing, segments: undefined };
    box.parts.matrix.show(frame);
  });
}

// value is a field's color: the local edit if there is one, else the status.
function value(target, app, f) {
  const k = id(target, app, f.key);
  if (local.has(k) && local.get(k) === f.value) local.delete(k); // saved and reported back
  return local.has(k) ? local.get(k) : f.value;
}

// withEdits applies local clock edits ("weekdayBar.activeColor") to a copy
// of the clock's settings, so the preview follows the picker.
function withEdits(clock, fields) {
  const out = JSON.parse(JSON.stringify(clock));
  for (const f of fields) {
    const v = value("clock", "", f);
    const path = f.key.split(".");
    let o = out;
    for (const p of path.slice(0, -1)) o = o[p] = o[p] || {};
    o[path[path.length - 1]] = v || null;
  }
  return out;
}

function rows(parent, fields, target, app, fallback) {
  keyed(parent, fields, f => f.key, f => createRow(target, app, f), (row, f) => {
    const p = row.parts;
    p.field = f;
    const v = value(target, app, f);
    const shown = v || fallback;
    p.label.textContent = f.label;
    p.hex.textContent = v ? v.toUpperCase() : "Default (" + fallback.toUpperCase() + ")";
    p.swatch.style.background = shown;
    if (document.activeElement !== p.input) p.input.value = shown.toLowerCase();
    // Only the clock's time and date can go back to the default text color.
    p.reset.hidden = !(target === "clock" && (f.key === "timeColor" || f.key === "dateColor") && v);
  });
}

function createRow(target, app, f) {
  const row = el("div", "color-row");
  const p = row.parts = { field: f, input: el("input"), swatch: el("label", "swatch"), label: el("div", "color-label"), hex: el("div", "color-hex") };
  p.input.type = "color";
  p.input.setAttribute("aria-label", (app ? pretty(app) + " " : "") + f.label + " color");
  p.swatch.append(p.input);
  const text = el("div", "color-text");
  text.append(p.label, p.hex);
  p.reset = el("button", "icon-btn");
  p.reset.type = "button";
  p.reset.title = "Back to the default text color";
  p.reset.append(icon("undo"));
  row.append(p.swatch, text, p.reset);

  const k = () => id(target, app, p.field.key);
  const box = () => row.closest(".app-colors");
  p.input.oninput = () => {
    local.set(k(), p.input.value.toUpperCase());
    if (box()) box().parts.editing = p.input.value;
    ui.status && render(ui.status);
  };
  p.input.onchange = () => save(target, app, p.field.key, p.input.value.toUpperCase(), k());
  p.reset.onclick = () => save(target, app, p.field.key, "", k());
  return row;
}

async function save(target, app, key, v, k) {
  local.set(k, v);
  try {
    await api.setColor(target, app, key, v);
    ui.toast(target === "clock" ? "Saved on the clock" : "Saved");
    if (target === "clock") ui.refresh(600);
    else ui.configChanged();
  } catch (e) {
    local.delete(k);
    ui.toast(e.message, "err");
  }
  for (const b of appList.children) b.parts.editing = null;
  setTimeout(() => local.delete(k), 8000); // never hold a stale edit for long
}
