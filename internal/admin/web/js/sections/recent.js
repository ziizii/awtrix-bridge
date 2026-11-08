// Recent switches, sounds and arm changes, newest first.

import { $, el, hhmmss } from "../dom.js";

export function render(s) {
  const events = (s.recent || []).slice().reverse();
  $("recent").replaceChildren(...(events.length
    ? events.map(p => {
      const li = el("li");
      const what = p.topic.split("/").pop() + " " + (p.payload || "");
      li.append(el("time", null, hhmmss(p.at)), el("span", null, p.app), el("code", null, what));
      return li;
    })
    : [el("li", "empty", "None yet.")]));
}
