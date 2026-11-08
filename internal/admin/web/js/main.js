// Polls the bridge status every second and lets each section render it.
//
// Adding a section: put its markup in index.html and a module in
// sections/ exporting render(status, ui), then list it below.

import { api } from "./api.js";
import * as header from "./sections/header.js";
import * as pickup from "./sections/pickup.js";
import * as chime from "./sections/chime.js";
import * as apps from "./sections/apps.js";
import * as recent from "./sections/recent.js";
import * as editor from "./sections/editor.js";

const sections = [header, pickup, chime, apps, recent];

// ui is what sections may ask of the page after an action.
const ui = {
  refresh: (ms = 0) => setTimeout(poll, ms),
  // A setting was written to the config file: show it in the editor too.
  configChanged: () => { editor.refreshIfClean(); ui.refresh(1200); },
};

async function poll() {
  let status;
  try { status = await api.status(); }
  catch (e) { header.unreachable(e); return; }
  for (const s of sections) {
    try { s.render(status, ui); }
    catch (e) { console.error(e); } // one broken section must not blank the page
  }
}

editor.init(ui);
poll();
setInterval(poll, 1000);
