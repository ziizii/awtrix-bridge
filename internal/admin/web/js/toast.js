// Short messages that slide in and go away on their own.

export function toast(message, kind = "") {
  const host = document.getElementById("toasts");
  const t = document.createElement("div");
  t.className = "toast " + kind;
  t.textContent = message;
  t.setAttribute("role", kind === "err" ? "alert" : "status");
  host.append(t);
  requestAnimationFrame(() => requestAnimationFrame(() => t.classList.add("in")));
  setTimeout(() => {
    t.classList.remove("in");
    setTimeout(() => t.remove(), 300);
  }, kind === "err" ? 6000 : 2600);
}
