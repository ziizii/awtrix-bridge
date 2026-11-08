// Commands the clock refuses right now (from its <topic>/result answers),
// e.g. an app payload with a key the firmware does not know. Hidden while
// everything is accepted; marks the Activity tab while not.

import { el, card, keyed, ago } from "../dom.js";
import { icon } from "../icons.js";

let root, list;

export function mount() {
  const c = card("Rejected by the clock", "It ignores these until they change");
  root = c.root;
  root.classList.add("rejected");
  list = el("ul", "events");
  root.append(list);
  root.hidden = true;
  return root;
}

export function render(s, ui) {
  const items = s.rejected || [];
  root.hidden = items.length === 0;
  ui.badge("activity", items.length > 0, "rejected");
  keyed(list, items, r => r.topic, () => {
    const li = el("li", "event bad");
    const ic = el("div", "ev-ic");
    ic.append(icon("alert"));
    li.parts = { title: el("div", "ev-title"), detail: el("div", "ev-detail"), time: el("time") };
    const main = el("div", "ev-main");
    main.append(li.parts.title, li.parts.detail);
    li.append(ic, main, li.parts.time);
    return li;
  }, (li, r) => {
    li.parts.title.textContent = r.reason;
    const app = r.topic.split("/pushed/")[1];
    li.parts.detail.textContent = (app ? "App " + app : r.topic) + (r.count > 1 ? ` · ${r.count} times` : "");
    li.title = r.topic;
    li.parts.time.textContent = ago(r.at);
  });
}
