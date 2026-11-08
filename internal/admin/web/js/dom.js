// Small DOM helpers shared by the sections.

import { toast } from "./toast.js";

export const $ = id => document.getElementById(id);

// el creates an element with an optional class and text.
export function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

export function button(text, onclick, cls = "btn") {
  const b = el("button", cls, text);
  b.type = "button";
  b.onclick = onclick;
  return b;
}

// card builds a section card with a title and an optional subtitle;
// head holds the title block and anything added to its right.
export function card(title, sub) {
  const root = el("div", "card");
  const head = el("div", "card-head");
  const text = el("div");
  text.append(el("h2", "card-title", title));
  const subEl = el("div", "card-sub", sub);
  subEl.hidden = sub == null;
  text.append(subEl);
  head.append(text);
  root.append(head);
  return { root, head, sub: subEl };
}

// keyed keeps one element per key inside parent, in the order of items:
// create(item) builds a missing element, update(element, item) refreshes
// it. Elements are kept rather than rebuilt because the page re-renders
// every second, and replacing a button swallows a click that lands
// mid-update.
export function keyed(parent, items, key, create, update) {
  const old = new Map([...parent.children].filter(c => c.dataset.key).map(c => [c.dataset.key, c]));
  [...parent.children].filter(c => !c.dataset.key).forEach(c => c.remove()); // placeholders
  items.forEach((item, i) => {
    const k = key(item);
    let node = old.get(k);
    if (!node) { node = create(item); node.dataset.key = k; }
    old.delete(k);
    update(node, item);
    if (parent.children[i] !== node) parent.insertBefore(node, parent.children[i] || null);
  });
  old.forEach(n => n.remove());
}

// setOptions fills a <select> with [value, label] pairs, only when they changed.
export function setOptions(select, pairs) {
  const sig = JSON.stringify(pairs);
  if (select.dataset.sig === sig) return;
  select.dataset.sig = sig;
  select.replaceChildren(...pairs.map(([value, label]) => {
    const o = el("option", null, label);
    o.value = value;
    return o;
  }));
}

// Run an action from a button: disable it meanwhile, report errors.
export async function busy(btn, fn) {
  btn.disabled = true;
  try { await fn(); } catch (e) { toast(e.message, "err"); }
  btn.disabled = false;
}

// pretty turns a config name into a label: "temp_out" -> "Temp out".
export const pretty = name => {
  const s = String(name || "").replace(/[_-]+/g, " ").trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
};

export function ago(t) {
  const s = Math.max(0, Math.round((Date.now() - new Date(t)) / 1000));
  if (s < 60) return s + "s ago";
  if (s < 3600) return Math.round(s / 60) + "m ago";
  if (s < 86400) return Math.round(s / 3600) + "h ago";
  return dateTime(t);
}

// Times are always 24-hour, whatever the browser's locale.
const two = n => String(n).padStart(2, "0");
export const hhmm = t => { const d = new Date(t); return two(d.getHours()) + ":" + two(d.getMinutes()); };
export const hhmmss = t => hhmm(t) + ":" + two(new Date(t).getSeconds());
export const dateTime = t => { const d = new Date(t); return `${d.getDate()}. ${d.getMonth() + 1}. ${d.getFullYear()} ${hhmm(d)}`; };
