// An inline color picker that stays open while colors are tried:
// LED-friendly presets, hue / saturation / brightness sliders and a hex
// field. Every change calls onLive at once; only Done and Cancel close it.
// (A browser's own <input type=color> closes after each pick on phones.)

import { el, button } from "./dom.js";

// Colors that look distinct on the clock's LEDs.
const PRESETS = [
  "#FFFFFF", "#FFD8A8", "#FF0000", "#FF4400", "#FF8800", "#FFB000", "#FFE000", "#AAFF00", "#00FF00",
  "#00FF99", "#00FFFF", "#00AAFF", "#0044FF", "#7700FF", "#FF00FF", "#FF0077", "#808080", "#303030",
];

const hex2 = n => Math.round(n).toString(16).padStart(2, "0");

export function hsvToHex(h, s, v) {
  s /= 100; v /= 100;
  const f = n => {
    const k = (n + h / 60) % 6;
    return v - v * s * Math.max(0, Math.min(k, 4 - k, 1));
  };
  return ("#" + hex2(f(5) * 255) + hex2(f(3) * 255) + hex2(f(1) * 255)).toUpperCase();
}

export function hexToHsv(hex) {
  const n = parseInt(hex.slice(1), 16);
  const r = (n >> 16) / 255, g = ((n >> 8) & 255) / 255, b = (n & 255) / 255;
  const max = Math.max(r, g, b), d = max - Math.min(r, g, b);
  let h = 0;
  if (d) h = max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
  return { h: (h * 60 + 360) % 360, s: max ? (d / max) * 100 : 0, v: max * 100 };
}

const isHex = v => /^#[0-9A-Fa-f]{6}$/.test(v);

// colorPanel returns { el, open(hex), value() }.
export function colorPanel({ onLive, onDone, onCancel }) {
  const root = el("div", "cpick");
  let cur = "#FFFFFF", hsv = hexToHsv(cur);

  const presets = el("div", "cpick-presets");
  for (const c of PRESETS) {
    const b = el("button", "cpick-preset");
    b.type = "button";
    b.style.background = c;
    b.title = c;
    b.setAttribute("aria-label", "Use " + c);
    b.onclick = () => set(c, true);
    presets.append(b);
  }

  const slider = (label, max) => {
    const input = el("input", "cpick-range");
    Object.assign(input, { type: "range", min: 0, max, step: 1 });
    input.setAttribute("aria-label", label);
    const row = el("label", "cpick-slider");
    row.append(el("span", null, label), input);
    return { row, input };
  };
  const H = slider("Hue", 359), S = slider("Saturation", 100), V = slider("Brightness", 100);
  H.input.classList.add("hue");
  for (const sl of [H, S, V]) {
    sl.input.oninput = () => {
      hsv = { h: +H.input.value, s: +S.input.value, v: +V.input.value };
      set(hsvToHex(hsv.h, hsv.s, hsv.v), false);
    };
  }

  const swatch = el("div", "cpick-swatch");
  const hexIn = el("input", "field cpick-hex");
  Object.assign(hexIn, { type: "text", maxLength: 7, spellcheck: false, autocapitalize: "characters" });
  hexIn.setAttribute("aria-label", "Hex color");
  hexIn.oninput = () => {
    let v = hexIn.value.trim().toUpperCase();
    if (v && v[0] !== "#") v = "#" + v;
    if (isHex(v)) set(v, true, false);
  };
  const done = button("Done", () => onDone(cur), "btn primary");
  const cancel = button("Cancel", () => onCancel(), "btn ghost");
  const foot = el("div", "cpick-foot");
  foot.append(swatch, hexIn, cancel, done);

  root.append(presets, H.row, S.row, V.row, foot);

  // set shows a color; fromHex re-derives the sliders (not while one is
  // dragged), writeHex refreshes the hex field (not while it is typed in).
  function set(c, fromHex, writeHex = true) {
    cur = c;
    if (fromHex) {
      hsv = hexToHsv(c);
      H.input.value = Math.round(hsv.h);
      S.input.value = Math.round(hsv.s);
      V.input.value = Math.round(hsv.v);
    }
    swatch.style.background = c;
    if (writeHex) hexIn.value = c;
    S.input.style.setProperty("--track", `linear-gradient(90deg, ${hsvToHex(hsv.h, 0, hsv.v)}, ${hsvToHex(hsv.h, 100, hsv.v)})`);
    V.input.style.setProperty("--track", `linear-gradient(90deg, #000, ${hsvToHex(hsv.h, hsv.s, 100)})`);
    for (const b of presets.children) b.classList.toggle("sel", b.title === c);
    onLive(c);
  }

  return {
    el: root,
    open(c) {
      cur = isHex(c) ? c.toUpperCase() : "#FFFFFF";
      hsv = hexToHsv(cur);
      H.input.value = Math.round(hsv.h);
      S.input.value = Math.round(hsv.s);
      V.input.value = Math.round(hsv.v);
      swatch.style.background = cur;
      hexIn.value = cur;
      S.input.style.setProperty("--track", `linear-gradient(90deg, ${hsvToHex(hsv.h, 0, hsv.v)}, ${hsvToHex(hsv.h, 100, hsv.v)})`);
      V.input.style.setProperty("--track", `linear-gradient(90deg, #000, ${hsvToHex(hsv.h, hsv.s, 100)})`);
      for (const b of presets.children) b.classList.toggle("sel", b.title === cur);
    },
    value: () => cur,
  };
}
