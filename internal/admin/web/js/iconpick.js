// An icon picker: the icon's preview, its LaMetric icon number, and Save
// while edited. Saving goes through the bridge, which uploads the icon to
// the clock if the clock does not have it yet.

import { el, button, busy } from "./dom.js";

export const ICON_GALLERY = "https://developer.lametric.com/icons";

// findIcons is the link to LaMetric's icon gallery.
export function findIcons() {
  const a = el("a", "find-icons", "Find icons ↗");
  a.href = ICON_GALLERY;
  a.target = "_blank";
  a.rel = "noopener";
  return a;
}

// iconPicker builds a row; save(id) stores the icon ("" = none). set(id)
// shows the saved icon unless the user is editing it.
export function iconPicker(label, save, ui) {
  const img = el("img", "icon-img");
  img.alt = "";
  img.onload = () => { img.hidden = false; };
  img.onerror = () => { img.hidden = true; };
  const box = el("div", "icon-box");
  box.append(img);
  const show = id => {
    img.hidden = !id;
    if (id) img.src = "https://developer.lametric.com/content/apps/icon_thumbs/" + id;
  };

  const input = el("input", "field icon-field");
  Object.assign(input, { type: "text", inputMode: "numeric", placeholder: "none", maxLength: 7 });
  input.setAttribute("aria-label", label + ": LaMetric icon number");
  let editing = false;
  const saveBtn = button("Save", () => busy(saveBtn, async () => {
    await save(input.value);
    editing = false;
    saveBtn.hidden = true;
    ui.toast(input.value ? "Icon saved, it is on the clock" : "Icon removed");
    ui.configChanged();
  }), "btn primary");
  saveBtn.hidden = true;
  input.oninput = () => {
    input.value = input.value.replace(/\D/g, "");
    editing = true;
    saveBtn.hidden = false;
    show(input.value);
  };

  const row = el("div", "form-row icon-row");
  const grow = el("div", "grow");
  grow.append(box, input, saveBtn);
  row.append(el("label", null, label), grow);
  return {
    el: row,
    set(id) {
      id = id || "";
      if (editing || document.activeElement === input || input.value === id) return;
      input.value = id;
      show(id);
    },
  };
}
