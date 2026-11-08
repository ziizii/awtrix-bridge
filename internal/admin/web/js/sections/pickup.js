// Pickup: countdowns with require_arm. Arm/disarm, the live countdown,
// and the alert (minutes before arrival + melody, saved to the config).

import { $, el, button, busy, keyed, setOptions } from "../dom.js";
import { api } from "../api.js";
import { led, setLed } from "../led.js";
import { melodyChoices, playable } from "../melodies.js";

export function render(s, ui) {
  const rows = (s.countdowns || []).filter(c => c.require_arm);
  $("pickup").hidden = rows.length === 0;
  const shown = new Map((s.apps || []).map(p => [p.app, p]));
  keyed($("pickup-rows"), rows, c => c.app, c => createRow(c, ui), (row, c) => updateRow(row, c, s, shown));
}

function createRow(c, ui) {
  const row = el("div", "arm");
  const p = row.parts = { item: c, state: el("div", "state"), led: led() };
  p.armBtn = button("", () => busy(p.armBtn, async () => {
    await api.arm(p.item.app, !p.item.armed);
    ui.refresh(300);
  }), "big");

  const info = el("div");
  info.append(el("div", "name", c.app), p.state);
  const controls = el("div", "controls");
  controls.append(p.led, p.armBtn);
  row.append(info, controls, createAlert(p, ui));
  return row;
}

// The alert inputs keep what the user typed until it is saved; the
// once-a-second refresh must not overwrite it.
// Element references live in row.parts, not on the element itself, where
// they could clash with DOM properties (a button already has .name).
function createAlert(p, ui) {
  p.minutes = el("input");
  Object.assign(p.minutes, { type: "number", min: 1, max: 120 });
  p.melody = el("select");
  p.save = button("Save", () => busy(p.save, async () => {
    await api.setAlert(p.item.app, Number(p.minutes.value), p.melody.value);
    p.editing = false;
    ui.configChanged();
  }));
  const play = button("▶", () => {
    if (playable(p.melody.value)) api.playMelody(p.melody.value).catch(e => alert(e.message));
  });
  play.title = "Preview on the clock";
  const edit = () => { p.editing = true; p.save.hidden = false; };
  p.minutes.oninput = edit;
  p.melody.onchange = edit;

  const alertBox = el("div", "alert");
  alertBox.append("Alert ", p.minutes, " min before, play ", p.melody, play, p.save,
    el("span", "hint", "· once per arming"));
  return alertBox;
}

function updateRow(row, c, s, shown) {
  const p = row.parts;
  p.item = c;
  p.state.replaceChildren(...(c.armed
    ? [el("b", null, "Armed"), " · on the clock now, disarms itself after the bus arrives"]
    : ["Off · hidden on the clock"]));
  const pub = shown.get(c.app);
  p.led.hidden = !(c.armed && pub && pub.payload);
  if (pub) setLed(p.led, pub.payload);
  p.armBtn.textContent = c.armed ? "Disarm" : "Arm for next bus";
  p.armBtn.classList.toggle("primary", !c.armed);

  if (!p.editing) {
    setOptions(p.melody, melodyChoices(s.melodies || [], "No alert", c.alert_melody).map(m => [m.name, m.label]));
    p.melody.value = c.alert_melody;
    p.minutes.value = c.alert_minutes || 5;
    p.save.hidden = true;
  }
}
