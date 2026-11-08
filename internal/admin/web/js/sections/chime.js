// Hourly chime: pick a melody (saved to the config), preview it on the
// clock, and see the quiet time that silences it.

import { $, el, button, busy, keyed, hhmm } from "../dom.js";
import { api } from "../api.js";
import { melodyChoices, playable } from "../melodies.js";

export function render(s, ui) {
  const melodies = s.melodies || [];
  $("chime").hidden = melodies.length === 0;
  const ch = s.chime || {};
  renderQuiet(ch.quiet);
  keyed($("chime-rows"), melodyChoices(melodies, "Off", ch.melody), m => m.name,
    m => createRow(m, ui),
    (row, m) => {
      const selected = m.name === ch.melody;
      row.classList.toggle("sel", selected);
      row.parts.use.hidden = selected || m.name === "custom";
      row.parts.tag.hidden = !selected;
    });
}

function renderQuiet(q) {
  const note = $("chime-quiet");
  if (!q) {
    note.textContent = "Plays every full hour, also at night (no quiet_entity set).";
    return;
  }
  const span = q.from && q.to ? " " + hhmm(q.from) + "–" + hhmm(q.to) : "";
  const now = q.on ? "quiet, chime is silent" : q.state === "off" ? "not quiet" : q.state;
  note.replaceChildren("Silent during quiet time" + span + " · now ", el("b", null, now),
    " (" + (q.name || q.entity) + ")");
  note.title = q.entity;
}

function createRow(m, ui) {
  const row = el("div", "mel");
  row.append(el("span", "lbl", m.label));
  if (playable(m.name)) {
    const play = button("▶ Play", () => api.playMelody(m.name).catch(e => alert(e.message)));
    play.title = "Preview on the clock";
    row.append(play);
  }
  const use = button("Use", () => busy(use, async () => {
    await api.setChime(m.name);
    ui.configChanged();
  }));
  row.parts = { use, tag: el("span", "tag", "Selected") };
  row.append(row.parts.use, row.parts.tag);
  return row;
}
