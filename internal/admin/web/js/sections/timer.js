// Timer: start one for a duration (presets or custom minutes) or until a
// time of day. While it runs: a live preview, when it ends, add time or
// cancel. The melody it plays at the end is saved to the config.

import { el, card, button, busy, setOptions, hhmm } from "../dom.js";
import { api } from "../api.js";
import { icon } from "../icons.js";
import { Matrix } from "../matrix.js";
import { clockApps } from "../frames.js";
import { melodyChoices, playable, OFF } from "../melodies.js";

const PRESETS = [1, 3, 5, 10, 15, 20, 30, 60]; // minutes
let p; // element references
let ui;

export function mount(u) {
  ui = u;
  const c = card("Timer", "Counts down on the clock, then plays a sound");
  p = { root: c.root, sub: c.sub, mode: "duration" };
  c.root.classList.add("timer");
  p.chip = el("span", "chip");
  p.chip.hidden = true;
  c.head.append(p.chip);

  p.setup = el("div", "timer-setup");
  p.setup.append(segmented(), durationPane(), untilPane());
  p.run = runPane();
  c.root.append(p.setup, p.run, melodyPicker());
  setMode("duration");
  return c.root;
}

// ---- idle: pick how ----

function segmented() {
  const seg = el("div", "seg");
  seg.setAttribute("role", "tablist");
  p.segBtns = {};
  for (const [mode, label] of [["duration", "Duration"], ["until", "Until a time"]]) {
    const b = button(label, () => setMode(mode), "seg-btn");
    b.setAttribute("role", "tab");
    p.segBtns[mode] = b;
    seg.append(b);
  }
  return seg;
}

function setMode(mode) {
  p.mode = mode;
  for (const [m, b] of Object.entries(p.segBtns)) b.setAttribute("aria-selected", m === mode);
  p.durationPane.hidden = mode !== "duration";
  p.untilPane.hidden = mode !== "until";
  if (mode === "until" && !p.atSet) setAt(suggestTime());
  updateUntilHint();
}

function durationPane() {
  const pane = p.durationPane = el("div", "timer-pane");
  const presets = el("div", "presets");
  for (const m of PRESETS) {
    const b = el("button", "preset");
    b.type = "button";
    b.append(el("span", "preset-n", m === 60 ? "1" : String(m)), el("span", "preset-u", m === 60 ? "hour" : "min"));
    b.setAttribute("aria-label", `Start a ${m} minute timer`);
    b.onclick = () => busy(b, () => start(() => api.startTimer(m * 60)));
    presets.append(b);
  }

  p.minutes = el("input");
  Object.assign(p.minutes, { type: "number", min: 1, max: 1440, value: 25, inputMode: "numeric" });
  p.minutes.setAttribute("aria-label", "Minutes");
  const step = d => () => { p.minutes.value = clampMinutes((Number(p.minutes.value) || 0) + d); };
  const stepper = el("div", "stepper");
  const minus = el("button"), plus = el("button");
  minus.type = plus.type = "button";
  minus.append(icon("minus"));
  plus.append(icon("plus"));
  minus.onclick = step(-1);
  plus.onclick = step(1);
  minus.setAttribute("aria-label", "One minute less");
  plus.setAttribute("aria-label", "One minute more");
  stepper.append(minus, p.minutes, plus);
  const go = button("Start", () => busy(go, () => start(() => api.startTimer(clampMinutes(p.minutes.value) * 60))), "btn primary");
  go.prepend(icon("play"));

  const custom = el("div", "form-row");
  const grow = el("div", "grow");
  grow.append(stepper, el("span", "unit", "min"));
  custom.append(grow, go);
  pane.append(presets, custom);
  return pane;
}

const clampMinutes = v => Math.min(1440, Math.max(1, Math.round(Number(v) || 1)));

function untilPane() {
  const pane = p.untilPane = el("div", "timer-pane");
  // Two 24-hour selects rather than <input type=time>, which follows the
  // browser's locale and may show AM/PM; phones show them as wheels.
  const two = n => String(n).padStart(2, "0");
  p.atH = el("select", "field time-part");
  p.atM = el("select", "field time-part");
  setOptions(p.atH, Array.from({ length: 24 }, (_, i) => [two(i), two(i)]));
  setOptions(p.atM, Array.from({ length: 12 }, (_, i) => [two(i * 5), two(i * 5)])); // 5-minute steps
  p.atH.setAttribute("aria-label", "End hour");
  p.atM.setAttribute("aria-label", "End minute");
  p.atH.onchange = p.atM.onchange = () => { p.atSet = true; updateUntilHint(); };
  const at = el("div", "time-pick");
  at.append(p.atH, el("span", "time-colon", ":"), p.atM);
  p.atHint = el("div", "until-hint");
  const go = button("Start", () => busy(go, () => {
    return start(() => api.startTimerAt(atValue()));
  }), "btn primary");
  go.prepend(icon("play"));
  const row = el("div", "form-row");
  const grow = el("div", "grow");
  grow.append(at);
  row.append(grow, go);
  pane.append(row, p.atHint);
  return pane;
}

const atValue = () => p.atH.value + ":" + p.atM.value;

function setAt(hhmmValue) {
  [p.atH.value, p.atM.value] = hhmmValue.split(":");
  p.atSet = true;
}

// suggestTime proposes the next quarter hour at least ten minutes away.
function suggestTime() {
  const t = new Date(Date.now() + 10 * 60000);
  t.setMinutes(Math.ceil(t.getMinutes() / 15) * 15, 0, 0);
  return hhmm(t);
}

// nextAt mirrors the bridge: the next time the clock shows hh:mm.
function nextAt(value) {
  const [h, m] = value.split(":").map(Number);
  const t = new Date();
  t.setHours(h, m, 0, 0);
  if (t <= new Date()) t.setDate(t.getDate() + 1);
  return t;
}

function updateUntilHint() {
  const t = nextAt(atValue());
  const tomorrow = t.getDate() !== new Date().getDate();
  p.atHint.textContent = "in " + span(t - Date.now()) + (tomorrow ? " · tomorrow" : "");
}

async function start(call) {
  await call();
  ui.toast("Timer started");
  ui.refresh(300);
}

// ---- running ----

function runPane() {
  const pane = el("div", "timer-run");
  pane.hidden = true;
  p.matrix = new Matrix();
  const screen = el("div", "screen");
  screen.append(p.matrix.el);
  p.left = el("div", "timer-left");
  p.ends = el("div", "timer-ends");
  const info = el("div", "timer-info");
  info.append(p.left, p.ends);
  p.bar = el("div", "timer-bar");
  p.fill = el("div", "timer-fill");
  p.bar.append(p.fill);

  const add = s => {
    const b = button("+" + (s / 60) + " min", () => busy(b, async () => {
      await api.extendTimer(s);
      ui.refresh(300);
    }));
    return b;
  };
  p.cancel = button("Cancel", () => busy(p.cancel, async () => {
    await api.cancelTimer();
    ui.refresh(300);
  }), "btn ghost danger");
  const actions = el("div", "timer-actions");
  actions.append(add(60), add(300), p.cancel);
  pane.append(screen, info, p.bar, actions);
  return pane;
}

// ---- end melody ----

function melodyPicker() {
  const det = el("details", "disclose");
  const sum = el("summary");
  p.melodyText = el("span", "disclose-text");
  sum.append(icon("bell"), p.melodyText, icon("chevron", "chev"));

  p.melody = el("select", "field");
  p.melody.setAttribute("aria-label", "Timer melody");
  const playBtn = el("button", "icon-btn");
  playBtn.type = "button";
  playBtn.title = "Preview on the clock";
  playBtn.append(icon("play"));
  playBtn.onclick = () => {
    if (playable(p.melody.value)) api.playMelody(p.melody.value).catch(e => ui.toast(e.message, "err"));
  };
  p.melody.onchange = () => busy(p.melody, async () => {
    await api.setTimerMelody(p.melody.value);
    ui.toast("Timer sound saved");
    ui.configChanged();
  });

  const row = el("div", "form-row");
  const grow = el("div", "grow");
  grow.append(p.melody, playBtn);
  row.append(el("label", null, "Melody"), grow);
  const body = el("div", "disclose-body");
  body.append(row, iconRow());
  det.append(sum, body);
  return det;
}

// iconRow picks the timer's icon by LaMetric icon number; the bridge
// uploads it to the clock if the clock does not have it yet.
function iconRow() {
  p.iconImg = el("img", "icon-img");
  p.iconImg.alt = "";
  p.iconImg.onerror = () => { p.iconImg.hidden = true; };
  p.iconImg.onload = () => { p.iconImg.hidden = false; };
  const box = el("div", "icon-box");
  box.append(p.iconImg);

  p.iconInput = el("input", "field icon-field");
  Object.assign(p.iconInput, { type: "text", inputMode: "numeric", placeholder: "none", maxLength: 7 });
  p.iconInput.setAttribute("aria-label", "LaMetric icon number");
  p.iconInput.oninput = () => {
    p.iconInput.value = p.iconInput.value.replace(/\D/g, "");
    p.iconEditing = true;
    p.iconSave.hidden = false;
    showIcon(p.iconInput.value);
  };
  p.iconSave = button("Save", () => busy(p.iconSave, async () => {
    await api.setTimerIcon(p.iconInput.value);
    p.iconEditing = false;
    p.iconSave.hidden = true;
    ui.toast(p.iconInput.value ? "Icon saved, it is on the clock" : "Icon removed");
    ui.configChanged();
  }), "btn primary");
  p.iconSave.hidden = true;

  const find = el("a", "hint", "Find icons ↗");
  find.href = "https://developer.lametric.com/icons";
  find.target = "_blank";
  find.rel = "noopener";

  const row = el("div", "form-row");
  const grow = el("div", "grow");
  grow.append(box, p.iconInput, p.iconSave);
  row.append(el("label", null, "Icon"), grow);
  const wrap = el("div", "icon-setting");
  wrap.append(row, find);
  return wrap;
}

function showIcon(id) {
  p.iconImg.hidden = !id;
  if (id) p.iconImg.src = "https://developer.lametric.com/content/apps/icon_thumbs/" + id;
}

// ---- render ----

export function render(s) {
  const t = s.timer;
  p.root.hidden = !t;
  if (!t) return;
  const now = Date.now(), start = new Date(t.start).getTime(), end = new Date(t.end).getTime();
  const done = t.running && now >= end;

  p.setup.hidden = t.running;
  p.run.hidden = !t.running;
  p.root.classList.toggle("running", t.running && !done);
  p.root.classList.toggle("done", done);
  ui.badge("now", t.running, "timer");
  p.chip.hidden = !t.running;
  p.sub.textContent = !t.running ? "Counts down on the clock, then plays a sound"
    : done ? "Counting up on the clock" : "Running on the clock";

  if (t.running) {
    const shown = clockApps(s).find(a => a.app === "timer");
    p.matrix.show(shown && !shown.frame.off ? shown.frame : { text: clockText(end - now) });
    p.chip.textContent = done ? "Done" : "Ends " + hhmm(t.end);
    p.chip.className = "chip " + (done ? "bad" : "accent");
    p.left.textContent = done ? "Time's up" : clockText(end - now);
    p.ends.textContent = done
      ? "Ended at " + hhmm(t.end) + " · clears in " + span(end + t.hold_after * 1000 - now)
      : "of " + span(end - start) + " · ends at " + hhmm(t.end);
    p.fill.style.width = (end > start ? Math.min(100, (100 * (now - start)) / (end - start)) : 100) + "%";
    p.cancel.lastChild.textContent = done ? "Dismiss" : "Cancel";
  } else if (p.mode === "until") {
    updateUntilHint();
  }

  const choices = melodyChoices(s.melodies || [], "No sound", t.melody);
  if (document.activeElement !== p.melody) {
    setOptions(p.melody, choices.map(m => [m.name, m.label.split(":")[0]]));
    p.melody.value = t.melody;
  }
  const cur = choices.find(m => m.name === t.melody);
  p.melodyText.textContent = (t.melody === OFF ? "No sound" : "Sound: " + (cur ? cur.label.split(":")[0] : t.melody))
    + " · " + (t.icon ? "Icon " + t.icon : "No icon");
  if (!p.iconEditing && document.activeElement !== p.iconInput && p.iconInput.value !== (t.icon || "")) {
    p.iconInput.value = t.icon || "";
    showIcon(p.iconInput.value);
  }
}

// clockText renders ms left like the clock: "4:05", "1:04:05".
function clockText(ms) {
  const s = Math.max(0, Math.ceil(ms / 1000));
  const mm = String(Math.floor(s / 60) % 60), ss = String(s % 60).padStart(2, "0");
  return s >= 3600 ? `${Math.floor(s / 3600)}:${mm.padStart(2, "0")}:${ss}` : `${Math.floor(s / 60)}:${ss}`;
}

// span renders a duration in words: "45 s", "12 min", "1 h 5 min".
function span(ms) {
  const m = Math.round(ms / 60000);
  if (ms < 60000) return Math.max(0, Math.round(ms / 1000)) + " s";
  if (m < 60) return m + " min";
  return Math.floor(m / 60) + " h" + (m % 60 ? " " + (m % 60) + " min" : "");
}
