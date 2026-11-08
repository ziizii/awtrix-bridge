// What the clock shows, as frames for matrix.js.
//
// A frame is { text, color, icon, segments, rainbow, weekday, progress,
// scroll }, { off: true } for a hidden app, or { setting, value } for a
// settings payload (brightness), which is not an app on the screen.
//
// The clock reports its own settings (status.clock, from its
// awtrix/state/settings topic): time format, colors, weekday bar and
// scrolling. Frames follow them, so the preview looks like the clock.

const pad = n => String(n).padStart(2, "0");
const MONTHS = ["JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"];
const DAYS = ["sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"];

// clockApps lists every app on the clock: the clock's built-in Time and
// Date, then what the bridge published. Time and Date frames are
// functions of the current time, so a matrix can keep them ticking.
export function clockApps(s) {
  const clock = s.clock || {};
  const builtins = [
    { app: "Time", builtin: true, frame: () => timeFrame(clock, new Date()) },
    { app: "Date", builtin: true, frame: () => dateFrame(clock, new Date()) },
  ];
  const pushed = (s.apps || []).map(p => ({
    ...p,
    pushed: p.topic.includes("/pushed/"),
    frame: frameOf(p.payload, clock),
  }));
  return builtins.concat(pushed);
}

// brightness is the brightness to show (0-255): the clock's own report,
// else what the bridge published, else null.
export function brightness(s) {
  if (s.clock && s.clock.brightness != null) return s.clock.brightness;
  const b = clockApps(s).find(p => p.frame.setting === "brightness");
  return b ? b.frame.value : null;
}

function timeFrame(c, now) {
  const h24 = c.time24h !== false;
  let h = now.getHours();
  if (!h24) h = h % 12 || 12;
  const hh = c.timeLeadingZero === false ? String(h) : pad(h);
  const blinkOff = c.timeSeparatorMode && c.timeSeparatorMode !== "steady" && now.getSeconds() % 2 === 1;
  const sep = blinkOff ? " " : ":";
  let text = hh + sep + pad(now.getMinutes());
  if (c.timeShowSeconds) text += sep + pad(now.getSeconds());
  if (!h24 && c.timeShowAmPm) text += now.getHours() < 12 ? "A" : "P";
  return { text, color: color(c.timeColor) || color(c.textColor), ...weekdayBar(c, now), scroll: scroll(c) };
}

function dateFrame(c, now) {
  const d = pad(now.getDate());
  const m = c.dateMonthNames ? MONTHS[now.getMonth()] : pad(now.getMonth() + 1);
  const y = c.dateYearMode === "none" ? null
    : c.dateYearMode === "fourDigit" ? String(now.getFullYear()) : pad(now.getFullYear() % 100);
  const order = { monthDayYear: [m, d, y], yearMonthDay: [y, m, d] }[c.dateOrder] || [d, m, y];
  const sep = c.dateMonthNames ? " " : ({ slash: "/", dash: "-", space: " " }[c.dateSeparator] || ".");
  const text = order.filter(p => p != null).join(sep);
  return { text, color: color(c.dateColor) || color(c.textColor), ...weekdayBar(c, now), scroll: scroll(c) };
}

// weekdayBar describes the bar under Time and Date: seven days with the
// clock's colors, today lit.
function weekdayBar(c, now) {
  const wb = c.weekdayBar || {};
  if (wb.show === false) return {};
  const first = wb.startOnMonday === false ? 0 : 1;
  const weekend = new Set(wb.weekendDays || []);
  const days = [];
  for (let i = 0; i < 7; i++) {
    const day = (first + i) % 7;
    const we = weekend.has(DAYS[day]);
    const today = day === now.getDay();
    const col = today
      ? (we && wb.weekendActiveColor) || wb.activeColor || "#ffffff"
      : (we && wb.weekendInactiveColor) || wb.inactiveColor || "#4a4a4a";
    days.push(color(col));
  }
  return { weekdays: days };
}

function scroll(c) {
  const s = c.scroll || {};
  return { wrap: s.mode === "wrap", gap: s.gap ?? 8, holdMs: s.holdMs ?? 1000, speed: s.speed ?? 100 };
}

export function frameOf(payload, clock = {}) {
  if (!payload) return { off: true };
  const base = { color: color(clock.textColor), scroll: scroll(clock) };
  let j;
  try { j = JSON.parse(payload); } catch { return { ...base, text: payload }; }
  if (typeof j !== "object" || j === null) return { ...base, text: String(j) };
  if (j.brightness != null && j.text == null) return { setting: "brightness", value: j.brightness };
  const f = { ...base, color: color(j.textColor ?? j.color) || base.color };
  if (Array.isArray(j.text)) f.segments = j.text.map(s => ({ t: s.t ?? "", c: color(s.c) || f.color }));
  else f.text = String(j.text ?? "");
  if (j.icon) f.icon = String(j.icon);
  if (j.rainbow) f.rainbow = true;
  if (j.progress != null && j.progress >= 0) {
    f.progress = Number(j.progress);
    f.progressColor = color(j.progressColor ?? j.progressC);
  }
  return f;
}

// color accepts the clock's forms: "#RRGGBB", [r, g, b] or a number.
function color(v) {
  if (Array.isArray(v)) return `rgb(${v.slice(0, 3).join(",")})`;
  if (typeof v === "number") return "#" + v.toString(16).padStart(6, "0");
  if (typeof v === "string" && v) return v.startsWith("#") ? v : "#" + v;
  return undefined;
}
