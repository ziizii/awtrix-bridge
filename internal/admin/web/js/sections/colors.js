// Look: colors and icons. The clock's own colors (time, date, weekday
// bar; saved on the clock), the timer's and each app's colors and icons
// (saved in config.yaml; icons are uploaded to the clock if missing).
//
// Picking a color opens a picker that stays open: the clock switches to
// what is being colored and shows every color tried (a preview, nothing
// saved) until Done saves it or Cancel puts the old one back. The status
// lists what can be changed (status.colors, see internal/bridge/colors.go).

import { el, card, keyed, pretty } from "../dom.js";
import { api } from "../api.js";
import { icon } from "../icons.js";
import { Matrix } from "../matrix.js";
import { clockApps } from "../frames.js";
import { iconPicker, findIcons } from "../iconpick.js";
import { colorPanel } from "../colorpick.js";

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
  timerPart = { run: new Matrix(), done: new Matrix(), rows: el("div", "color-rows"), icon: iconPicker("Icon", id => api.setTimerIcon(id), ui) };
  const pair = el("div", "screen-pair");
  pair.append(screen(timerPart.run), screen(timerPart.done));
  timer.root.append(pair, timerPart.icon.el, timerPart.rows);

  const apps = card("Apps", "Icons are LaMetric numbers. Thresholds pick a color by value.");
  apps.head.append(findIcons());
  timer.head.append(findIcons());
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
  timerPart.icon.set(s.timer && s.timer.icon);
  const t = Object.fromEntries(colors.timer.map(f => [f.key, value("timer", "", f) || textColor]));
  const ic = (s.timer && s.timer.icon) || undefined;
  timerPart.run.show({ text: "4:05", icon: ic, color: t.color, progress: 40, progressColor: t.progress_color, progressTrack: t.track_color });
  timerPart.done.show({ text: "+0:12", icon: ic, color: t.done_color });

  // Apps: the live payload, recolored while a picker is dragged.
  const live = new Map(clockApps(s).map(a => [a.app, a]));
  keyed(appList, colors.apps, a => a.app, a => {
    const box = el("div", "app-colors");
    box.parts = { matrix: new Matrix(), rows: el("div", "color-rows"), name: el("div", "app-colors-name", pretty(a.app)), icons: {} };
    const head = el("div", "app-colors-head");
    head.append(screen(box.parts.matrix), box.parts.name);
    const icons = el("div", "app-icons");
    for (const f of a.icons || []) {
      const pick = iconPicker(f.label, id => api.setAppIcon(a.app, f.key, id), ui);
      box.parts.icons[f.key] = pick;
      icons.append(pick.el);
    }
    box.append(head, icons, box.parts.rows);
    return box;
  }, (box, a) => {
    rows(box.parts.rows, a.fields, "app", a.app, textColor);
    for (const f of a.icons || []) box.parts.icons[f.key] && box.parts.icons[f.key].set(f.value);
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
  keyed(parent, fields, f => f.key, f => createRow(target, app, f), (item, f) => {
    const p = item.parts;
    p.field = f;
    p.fallback = fallback;
    const v = value(target, app, f);
    p.label.textContent = f.label;
    p.hex.textContent = v ? v.toUpperCase() : "Default (" + fallback.toUpperCase() + ")";
    p.swatch.style.background = v || fallback;
    // Only the clock's time and date can go back to the default text color.
    p.reset.hidden = item === open || !(target === "clock" && (f.key === "timeColor" || f.key === "dateColor") && v);
  });
}

// ---- picking: the picker stays open while colors are tried on the clock ----

let open = null; // the item whose picker is open

function createRow(target, app, f) {
  const item = el("div", "color-item");
  const p = item.parts = { field: f, target, app, swatch: el("button", "swatch"), label: el("div", "color-label"), hex: el("div", "color-hex") };
  p.swatch.type = "button";
  p.swatch.setAttribute("aria-label", "Pick " + (app ? pretty(app) + " " : "") + f.label + " color");
  const text = el("div", "color-text");
  text.append(p.label, p.hex);
  p.reset = el("button", "icon-btn");
  p.reset.type = "button";
  p.reset.title = "Back to the default text color";
  p.reset.append(icon("undo"));
  const row = el("div", "color-row");
  row.append(p.swatch, text, p.reset);

  p.panel = colorPanel({
    onLive: c => live(item, c),
    onDone: c => finish(item, c),
    onCancel: () => cancel(item),
  });
  p.panel.el.hidden = true;
  item.append(row, p.panel.el);

  p.swatch.onclick = () => (open === item ? finish(item, p.panel.value()) : start(item));
  text.onclick = p.swatch.onclick;
  p.reset.onclick = async () => {
    try {
      await api.setColor(target, app, p.field.key, "");
      ui.toast("Back to the default text color");
      ui.refresh(600);
    } catch (e) { ui.toast(e.message, "err"); }
  };
  return item;
}

const k = item => id(item.parts.target, item.parts.app, item.parts.field.key);

// start opens the picker and puts what is being colored on the clock.
async function start(item) {
  if (open) await finish(open, open.parts.panel.value());
  const p = item.parts;
  open = item;
  p.original = value(p.target, p.app, p.field);
  p.sent = p.original;
  p.prevActive = ui.status && ui.status.active;
  p.panel.open(p.original || p.fallback);
  p.panel.el.hidden = false;
  item.classList.add("open");
  p.reset.hidden = true;
  p.panel.el.scrollIntoView({ block: "nearest", behavior: "smooth" });

  try {
    if (p.target === "timer") {
      // The timer shows a sample while colors are picked: preview first so
      // the clock has the app, then switch to it.
      await api.previewColor("timer", "", p.field.key, p.original || p.fallback);
      await api.switchTo("timer");
    } else {
      const app = p.target === "clock" ? (p.field.key === "dateColor" ? "Date" : "Time") : p.app;
      const shown = clockApps(ui.status).find(a => a.app === app);
      if (shown && !shown.frame.off && ui.status.active !== app) await api.switchTo(app);
      else if (shown && shown.frame.off) ui.toast(pretty(app) + " is hidden on the clock right now");
    }
  } catch (e) { ui.toast(e.message, "err"); }
}

// live shows a color on the page at once and on the clock as fast as the
// bridge takes it: one request in flight, then the latest color.
function live(item, c) {
  const p = item.parts;
  local.set(k(item), c);
  const box = item.closest(".app-colors");
  if (box) box.parts.editing = c;
  if (ui.status) render(ui.status);
  p.want = c;
  if (!p.flight) p.flight = pump(item);
}

async function pump(item) {
  const p = item.parts;
  while (p.want && p.want !== p.sent) {
    const c = p.want;
    try {
      await api.previewColor(p.target, p.app, p.field.key, c);
      p.sent = c;
    } catch (e) {
      ui.toast(e.message, "err");
      break;
    }
  }
  p.flight = null;
}

function close(item) {
  const p = item.parts;
  p.panel.el.hidden = true;
  item.classList.remove("open");
  p.want = null;
  const box = item.closest(".app-colors");
  if (box) box.parts.editing = null;
  if (open === item) open = null;
}

// finish saves the color (Done, or opening another picker).
async function finish(item, c) {
  const p = item.parts;
  close(item);
  if (p.flight) await p.flight;
  const key = k(item);
  try {
    if (c !== p.original) {
      local.set(key, c);
      await api.setColor(p.target, p.app, p.field.key, c);
      ui.toast(p.target === "clock" ? "Saved on the clock" : "Saved");
      if (p.target === "clock") ui.refresh(600);
      else ui.configChanged(); // the reload also ends the preview
      setTimeout(() => local.delete(key), 8000); // never hold a stale edit for long
    } else {
      local.delete(key);
      if (p.target !== "clock") await api.endPreview(p.target, p.app);
    }
    if (p.target === "timer") await switchBack(p);
  } catch (e) {
    local.delete(key);
    ui.toast(e.message, "err");
  }
}

// cancel puts the old color back.
async function cancel(item) {
  const p = item.parts;
  close(item);
  if (p.flight) await p.flight;
  local.delete(k(item));
  if (ui.status) render(ui.status);
  try {
    if (p.target === "clock") {
      if (p.sent !== p.original) await api.previewColor("clock", "", p.field.key, p.original);
    } else {
      await api.endPreview(p.target, p.app);
    }
    if (p.target === "timer") await switchBack(p);
  } catch (e) { ui.toast(e.message, "err"); }
}

// switchBack leaves the timer's sample for what was on screen before.
async function switchBack(p) {
  const back = p.prevActive && p.prevActive !== "timer" ? p.prevActive : "Time";
  if (!(ui.status && ui.status.timer && ui.status.timer.running)) await api.switchTo(back);
}
