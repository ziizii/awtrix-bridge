// Builds the page from views.js, routes between views by the URL hash,
// and polls the bridge status every second for the sections to render.

import { api } from "./api.js";
import { $, el } from "./dom.js";
import { icon } from "./icons.js";
import { toast } from "./toast.js";
import { views } from "./views.js";
import * as conn from "./sections/conn.js";
import { refreshIfClean } from "./sections/editor.js";

const links = new Map(); // view id -> { a, badge }
const panes = new Map(); // view id -> element
const sections = [];

// ui is what sections may ask of the page.
const ui = {
  status: null, // the latest status
  refresh: (ms = 0) => setTimeout(poll, ms),
  // A setting was written to the config file: show it in the editor too.
  configChanged: () => { refreshIfClean(); ui.refresh(1200); },
  toast,
  // badge marks a view's tab, e.g. for unsaved edits or something armed;
  // source tells apart sections that mark the same view.
  badge: (view, on, source = view) => {
    const l = links.get(view);
    if (!l) return;
    if (on) l.on.add(source); else l.on.delete(source);
    l.badge.hidden = l.on.size === 0;
  },
  go: view => { location.hash = "#/" + view; },
};

function build() {
  for (const v of views) {
    const a = el("a");
    a.href = "#/" + v.id;
    const badge = el("span", "badge");
    badge.hidden = true;
    a.append(icon(v.icon), el("span", null, v.title), badge);
    $("nav").append(a);
    links.set(v.id, { a, badge, on: new Set() });

    const pane = el("section", "view");
    pane.hidden = true;
    pane.setAttribute("aria-label", v.title);
    pane.append(el("h1", "view-title", v.title));
    for (const s of v.sections) {
      pane.append(s.mount(ui));
      sections.push(s);
    }
    $("views").append(pane);
    panes.set(v.id, pane);
  }
}

function route() {
  const want = location.hash.replace(/^#\/?/, "");
  const cur = panes.has(want) ? want : views[0].id;
  const changed = panes.get(cur).hidden;
  for (const [id, pane] of panes) pane.hidden = id !== cur;
  for (const [id, { a }] of links) {
    if (id === cur) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  document.title = views.find(v => v.id === cur).title + " · Clock";
  if (changed) window.scrollTo(0, 0);
}

async function poll() {
  let status;
  try { status = await api.status(); }
  catch (e) { conn.unreachable(e); return; }
  ui.status = status;
  conn.render(status);
  for (const s of sections) {
    if (!s.render) continue;
    try { s.render(status, ui); }
    catch (e) { console.error(e); } // one broken section must not blank the page
  }
}

build();
addEventListener("hashchange", route);
route();
poll();
setInterval(poll, 1000);
