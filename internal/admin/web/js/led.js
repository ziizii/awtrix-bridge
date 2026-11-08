// Renders a published clock payload as an LED-style preview.

import { el } from "./dom.js";

export function led() {
  return el("div", "led");
}

// setLed shows what an app payload looks like on the clock: its text in
// its color, the brightness for settings, or "hidden" for a removed app.
export function setLed(node, payload) {
  node.className = "led";
  node.style.color = "";
  if (!payload) {
    node.classList.add("off");
    node.textContent = "hidden";
    return;
  }
  try {
    const j = JSON.parse(payload);
    if (j.text != null) {
      node.textContent = j.text;
      if (j.textColor) node.style.color = j.textColor;
    } else if (j.brightness != null) {
      node.textContent = "☀ " + j.brightness;
    } else {
      node.textContent = payload;
    }
  } catch {
    node.textContent = payload;
  }
}
