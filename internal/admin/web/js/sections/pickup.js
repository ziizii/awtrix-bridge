// Pickup: countdowns with require_arm. Arm/disarm, the live countdown,
// and the alert (minutes before arrival + melody, saved to the config).

import { el, button, busy, keyed, setOptions, pretty } from "../dom.js";
import { api } from "../api.js";
import { icon } from "../icons.js";
import { Matrix } from "../matrix.js";
import { clockApps } from "../frames.js";
import { melodyChoices, playable, OFF } from "../melodies.js";

let list;

export function mount() {
  list = el("div", "stack");
  list.hidden = true;
  return list;
}

export function render(s, ui) {
  const rows = (s.countdowns || []).filter(c => c.require_arm);
  list.hidden = rows.length === 0;
  ui.badge("now", rows.some(c => c.armed));
  const shown = new Map(clockApps(s).map(p => [p.app, p]));
  keyed(list, rows, c => c.app, c => createCard(c, ui), (root, c) => updateCard(root, c, s, shown));
}

// Element references live in root.parts, not on the elements themselves,
// where they could clash with DOM properties (a button already has .name).
function createCard(c, ui) {
  const root = el("div", "card pickup");
  const p = root.parts = { item: c, matrix: new Matrix() };

  const head = el("div", "pickup-head");
  const ic = el("div", "pickup-ic");
  ic.append(icon("bus"));
  const text = el("div", "pickup-text");
  p.state = el("div", "pickup-state");
  text.append(el("div", "pickup-title", pretty(c.app)), p.state);
  p.pill = el("span", "chip");
  head.append(ic, text, p.pill);

  p.screen = el("div", "screen");
  p.screen.append(p.matrix.el);

  p.armBtn = button("", () => busy(p.armBtn, async () => {
    await api.arm(p.item.app, !p.item.armed);
    ui.refresh(300);
  }), "btn lg block");

  root.append(head, p.screen, p.armBtn, createAlert(p, ui));
  return root;
}

// The alert inputs keep what the user typed until it is saved; the
// once-a-second refresh must not overwrite it.
function createAlert(p, ui) {
  const det = el("details", "disclose");
  const sum = el("summary");
  p.alertText = el("span", "disclose-text");
  sum.append(icon("bell"), p.alertText, icon("chevron", "chev"));

  const edit = () => { p.editing = true; p.save.hidden = false; };

  p.minutes = el("input");
  Object.assign(p.minutes, { type: "number", min: 1, max: 120, inputMode: "numeric" });
  p.minutes.setAttribute("aria-label", "Minutes before arrival");
  p.minutes.oninput = edit;
  const step = d => () => {
    p.minutes.value = Math.min(120, Math.max(1, (Number(p.minutes.value) || 0) + d));
    edit();
  };
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

  p.melody = el("select", "field");
  p.melody.setAttribute("aria-label", "Alert melody");
  p.melody.onchange = edit;
  const play = el("button", "icon-btn");
  play.type = "button";
  play.title = "Preview on the clock";
  play.append(icon("play"));
  play.onclick = () => {
    if (playable(p.melody.value)) api.playMelody(p.melody.value).catch(e => ui.toast(e.message, "err"));
  };

  p.save = button("Save alert", () => busy(p.save, async () => {
    await api.setAlert(p.item.app, Number(p.minutes.value), p.melody.value);
    p.editing = false;
    p.save.hidden = true;
    ui.toast("Alert saved");
    ui.configChanged();
  }), "btn primary block");
  p.save.hidden = true;

  const minutesRow = el("div", "form-row");
  minutesRow.append(el("label", null, "Minutes before"), stepper);
  const melodyRow = el("div", "form-row");
  const grow = el("div", "grow");
  grow.append(p.melody, play);
  melodyRow.append(el("label", null, "Melody"), grow);

  const body = el("div", "disclose-body");
  body.append(minutesRow, melodyRow, el("p", "hint", "Plays once per arming, when the bus is that close."), p.save);
  det.append(sum, body);
  return det;
}

function updateCard(root, c, s, shown) {
  const p = root.parts;
  p.item = c;
  root.classList.toggle("armed", c.armed);
  p.pill.textContent = c.armed ? "Armed" : "Off";
  p.pill.className = "chip" + (c.armed ? " accent" : "");
  p.state.textContent = c.armed
    ? "On the clock. Disarms itself after the bus arrives."
    : "Hidden on the clock until you arm it.";

  const pub = shown.get(c.app);
  p.screen.hidden = !(c.armed && pub && !pub.frame.off);
  if (pub) p.matrix.show(pub.frame);

  if (p.armed !== c.armed) { // rebuild the button only when it changes
    p.armed = c.armed;
    p.armBtn.replaceChildren(icon(c.armed ? "power" : "bus"), c.armed ? "Disarm" : "Arm for next bus");
    p.armBtn.classList.toggle("primary", !c.armed);
  }

  const choices = melodyChoices(s.melodies || [], "No alert", c.alert_melody);
  const label = (choices.find(m => m.name === c.alert_melody) || {}).label || c.alert_melody;
  p.alertText.textContent = c.alert_melody === OFF || !c.alert_minutes
    ? "No alert"
    : `Alert ${c.alert_minutes} min before · ${label.split(":")[0]}`;

  if (!p.editing) {
    setOptions(p.melody, choices.map(m => [m.name, m.label.split(":")[0]]));
    p.melody.value = c.alert_melody;
    p.minutes.value = c.alert_minutes || 5;
  }
}
