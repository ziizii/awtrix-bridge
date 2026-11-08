// The YAML config editor: check, save & reload, revert, reload from file.

import { $ } from "../dom.js";
import { api } from "../api.js";

let saved = ""; // the file content the editor started from

const text = () => $("yaml");
const dirty = () => text().value !== saved;

function say(msg, kind) {
  $("msg").textContent = msg;
  $("msg").className = "msg " + (kind || "");
}

async function load() {
  saved = await api.config();
  text().value = saved;
  say("");
}

// refreshIfClean reloads the file after another section changed it,
// unless the editor holds unsaved edits.
export async function refreshIfClean() {
  if (!dirty()) await load();
}

export function init(ui) {
  async function save() {
    $("save").disabled = true;
    say("Saving…");
    try {
      await api.saveConfig(text().value);
      saved = text().value;
      say("Saved and reloaded.", "ok");
      ui.refresh(1500);
    } catch (e) { say("Not saved: " + e.message, "err"); }
    $("save").disabled = false;
  }

  $("save").onclick = save;
  $("check").onclick = async () => {
    try { await api.check(text().value); say("Config is valid.", "ok"); }
    catch (e) { say(e.message, "err"); }
  };
  $("revert").onclick = () => { text().value = saved; say(""); };
  $("reload").onclick = async () => {
    if (dirty() && !confirm("Discard your edits and reload the file?")) return;
    try { await api.reload(); await load(); say("Reloaded from file.", "ok"); ui.refresh(1500); }
    catch (e) { say(e.message, "err"); }
  };

  text().addEventListener("input", () => say(dirty() ? "Unsaved changes" : "", dirty() ? "warn" : ""));
  text().addEventListener("keydown", e => {
    if ((e.metaKey || e.ctrlKey) && e.key === "s") { e.preventDefault(); save(); }
    if (e.key === "Tab") { e.preventDefault(); document.execCommand("insertText", false, "  "); }
  });
  window.addEventListener("beforeunload", e => { if (dirty()) e.preventDefault(); });

  load().catch(e => say(e.message, "err"));
}
