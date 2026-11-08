// The YAML config editor: check, save & reload, revert, reload from file.

import { el, card, button } from "../dom.js";
import { api } from "../api.js";
import { icon } from "../icons.js";

let saved = ""; // the file content the editor started from
let text, gutter, msg, ui;

const dirty = () => text.value !== saved;

function say(m, kind) {
  msg.textContent = m;
  msg.className = "msg " + (kind || "");
}

// The line numbers follow the text and its scroll position.
function sync() {
  const n = text.value.split("\n").length;
  if (gutter.dataset.n !== String(n)) {
    gutter.dataset.n = n;
    gutter.textContent = Array.from({ length: n }, (_, i) => i + 1).join("\n");
  }
  gutter.scrollTop = text.scrollTop;
  ui.badge("config", dirty());
}

async function load() {
  saved = await api.config();
  text.value = saved;
  say("");
  sync();
}

// refreshIfClean reloads the file after another section changed it,
// unless the editor holds unsaved edits.
export async function refreshIfClean() {
  if (text && !dirty()) await load();
}

export function mount(u) {
  ui = u;
  const c = card("config.yaml", "Saving reloads the bridge right away");

  gutter = el("pre", "gutter");
  gutter.setAttribute("aria-hidden", "true");
  text = el("textarea");
  Object.assign(text, { spellcheck: false, autocapitalize: "off", autocomplete: "off", wrap: "off" });
  text.setAttribute("aria-label", "Configuration");
  const code = el("div", "code");
  code.append(gutter, text);

  msg = el("div", "msg");
  const withIcon = (b, name) => {
    b.replaceChildren(icon(name), el("span", "btn-label", b.textContent));
    return b;
  };
  const save = withIcon(button("Save", () => doSave(save), "btn primary"), "save");
  const check = withIcon(button("Check", doCheck), "check");
  const revert = withIcon(button("Revert", () => { text.value = saved; say(""); sync(); }, "btn ghost"), "undo");
  revert.title = "Discard your edits";
  revert.setAttribute("aria-label", "Revert");
  const reload = withIcon(button("Reload", doReload, "btn ghost"), "refresh");
  reload.title = "Reload the file from disk and apply it";
  reload.setAttribute("aria-label", "Reload from file");
  const bar = el("div", "editor-bar");
  revert.classList.add("push");
  bar.append(msg, save, check, revert, reload);

  c.root.append(code, bar);

  text.addEventListener("input", () => {
    say(dirty() ? "Unsaved changes" : "", dirty() ? "warn" : "");
    sync();
  });
  text.addEventListener("scroll", () => { gutter.scrollTop = text.scrollTop; });
  text.addEventListener("keydown", e => {
    if ((e.metaKey || e.ctrlKey) && e.key === "s") { e.preventDefault(); doSave(save); }
    if (e.key === "Tab") { e.preventDefault(); document.execCommand("insertText", false, "  "); }
  });
  window.addEventListener("beforeunload", e => { if (dirty()) e.preventDefault(); });

  load().catch(e => say(e.message, "err"));
  return c.root;
}

async function doSave(btn) {
  btn.disabled = true;
  say("Saving…");
  try {
    await api.saveConfig(text.value);
    saved = text.value;
    say("");
    sync();
    ui.toast("Saved and reloaded");
    ui.refresh(1500);
  } catch (e) { say("Not saved: " + e.message, "err"); }
  btn.disabled = false;
}

async function doCheck() {
  try { await api.check(text.value); say("Config is valid.", "ok"); }
  catch (e) { say(e.message, "err"); }
}

async function doReload() {
  if (dirty() && !confirm("Discard your edits and reload the file?")) return;
  try {
    await api.reload();
    await load();
    ui.toast("Reloaded from file");
    ui.refresh(1500);
  } catch (e) { say(e.message, "err"); }
}
