// A 32×8 LED matrix drawn on a canvas, like the clock's screen.
//
// show(frame) draws a frame (see frames.js): an 8×8 icon on the left,
// text in the clock's pixel font, centered or, when too long, scrolling
// like the clock's scroll settings say, and an optional weekday or
// progress bar. A frame can also be a function returning one (the time),
// which the matrix re-reads a few times a second. The picture is a plain
// array of 256 colors, so a live copy of the clock's screen could be
// drawn the same way.

import { el } from "./dom.js";
import { glyph, normalize } from "./pixfont.js";

export const W = 32, H = 8;
const UNLIT = "rgba(255,255,255,0.065)";
const DIM = "#4a4a4a";
const STEP_MS = 60;   // one pixel of scrolling at the clock's speed 100

const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");

// Matrices that change by themselves (scrolling text, a ticking time),
// advanced by one shared timer.
const moving = new Set();
setInterval(() => {
  for (const m of moving) {
    if (!m.el.isConnected) { moving.delete(m); continue; }
    if (m.source) m.apply(m.source());
    if (m.scrolls) m.paint();
  }
}, 40);

export class Matrix {
  constructor({ glow = false } = {}) {
    this.el = el("div", "matrix");
    this.canvas = el("canvas");
    if (glow) {
      this.glow = el("canvas", "glow-layer");
      this.glow.width = W;
      this.glow.height = H;
      this.el.append(this.glow);
    }
    this.el.append(this.canvas);
    this.frame = { off: true };
    this.sig = "";
    this.source = null;  // a frame function, re-read by the timer
    this.scrolls = false;
    this.start = 0;      // when the text appeared, for scrolling
    this.px = new Array(W * H).fill(null);
    new ResizeObserver(() => this.resize()).observe(this.el);
  }

  show(frame) {
    this.source = typeof frame === "function" ? frame : null;
    this.apply(this.source ? frame() : frame);
    this.track();
  }

  apply(frame) {
    const sig = JSON.stringify(frame);
    if (sig === this.sig) return;
    if (frame.text !== this.frame.text) this.start = performance.now();
    this.sig = sig;
    this.frame = frame;
    if (frame.icon) loadIcon(frame.icon, () => this.paint());
    this.paint();
  }

  paint() {
    const { px, scrolls } = compose(this.frame, performance.now() - this.start);
    this.scrolls = scrolls;
    this.px = px;
    this.track();
    this.draw();
  }

  track() {
    if (this.source || this.scrolls) moving.add(this);
    else moving.delete(this);
  }

  resize() {
    const r = this.el.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    this.canvas.width = Math.round(r.width * dpr);
    this.canvas.height = Math.round(r.height * dpr);
    this.draw();
  }

  draw() {
    const c = this.canvas;
    if (!c.width || !c.height) return; // not visible yet
    const ctx = c.getContext("2d");
    const cw = c.width / W, ch = c.height / H;
    const gap = Math.max(1, cw * 0.16), r = cw * 0.2;
    ctx.clearRect(0, 0, c.width, c.height);
    // One path per color keeps this cheap enough to run every step.
    const byColor = new Map();
    this.px.forEach((color, i) => {
      const k = color || UNLIT;
      if (!byColor.has(k)) byColor.set(k, []);
      byColor.get(k).push(i);
    });
    for (const [color, cells] of byColor) {
      ctx.fillStyle = color;
      ctx.beginPath();
      for (const i of cells) {
        const x = (i % W) * cw + gap / 2, y = Math.floor(i / W) * ch + gap / 2;
        if (ctx.roundRect) ctx.roundRect(x, y, cw - gap, ch - gap, r);
        else ctx.rect(x, y, cw - gap, ch - gap);
      }
      ctx.fill();
    }
    if (this.glow) {
      const g = this.glow.getContext("2d");
      g.clearRect(0, 0, W, H);
      this.px.forEach((color, i) => {
        if (!color) return;
        g.fillStyle = color;
        g.fillRect(i % W, Math.floor(i / W), 1, 1);
      });
    }
  }
}

// compose turns a frame into 256 colors (null = off), elapsed ms after
// its text appeared.
function compose(f, elapsed) {
  const px = new Array(W * H).fill(null);
  const set = (x, y, color) => { if (x >= 0 && x < W && y >= 0 && y < H) px[y * W + x] = color; };
  if (!f || f.off) return { px, scrolls: false };

  let x0 = 0;
  if (f.icon) {
    x0 = 9; // the clock keeps this space even while an icon loads
    const icon = iconPixels(f.icon);
    if (icon) icon.forEach((color, i) => set(i % 8, Math.floor(i / 8), color));
  }
  const area = W - x0;

  const chars = [];
  for (const seg of f.segments || [{ t: f.text, c: f.color }]) {
    for (const ch of normalize(seg.t ?? "")) chars.push({ rows: glyph(ch), color: seg.c || "#ffffff" });
  }
  if (f.rainbow) chars.forEach((c, i) => { c.color = `hsl(${(i * 360) / Math.max(chars.length, 1)} 100% 58%)`; });
  const width = chars.reduce((w, c) => w + c.rows[0].length + 1, 0) - (chars.length ? 1 : 0);

  const text = x => {
    for (const c of chars) {
      c.rows.forEach((row, ry) => {
        for (let rx = 0; rx < row.length; rx++) {
          if (row[rx] === "1" && x + rx >= x0) set(x + rx, 1 + ry, c.color);
        }
      });
      x += c.rows[0].length + 1;
    }
  };

  // Text that fits is centered. Longer text rests at the start, then
  // scrolls left: round and round with a gap ("wrap" mode), or out and
  // back in from the right.
  const sc = f.scroll || {};
  const scrolls = width > area && !reducedMotion.matches;
  if (!scrolls) {
    text(width > area ? x0 : x0 + Math.floor((area - width) / 2));
  } else {
    const step = (STEP_MS * 100) / (sc.speed || 100);
    const holdMs = sc.holdMs ?? 1000;
    if (sc.wrap) {
      const period = width + (sc.gap ?? 8);
      const k = Math.floor(Math.max(0, elapsed - holdMs) / step) % period;
      text(x0 - k);
      text(x0 - k + period);
    } else {
      const hold = Math.round(holdMs / step);
      const cycle = hold + width + area;
      const k = Math.max(0, (Math.floor(elapsed / step) % cycle) - hold);
      text(k <= width ? x0 - k : x0 + area - (k - width));
    }
  }

  if (f.weekdays) {
    // Seven bars along the bottom, colored by the frame.
    const start = x0 ? x0 + Math.floor((area - 20) / 2) : 2, w = x0 ? 2 : 3;
    f.weekdays.forEach((col, d) => {
      for (let i = 0; i < w; i++) set(start + d * (w + 1) + i, 7, col);
    });
  } else if (f.progress != null) {
    const n = Math.round((Math.min(100, Math.max(0, f.progress)) / 100) * area);
    for (let i = 0; i < area; i++) set(x0 + i, 7, i < n ? f.progressColor || "#ffffff" : f.progressTrack || DIM);
  }
  return { px, scrolls };
}

// ---- icons ----
// The clock's icons are LaMetric icon IDs; their 8×8 images are public
// (with CORS), so the preview can read their pixels.

const icons = new Map(); // id -> { px, waiting }

function iconPixels(id) {
  const e = icons.get(id);
  return e ? e.px : null;
}

function loadIcon(id, done) {
  if (!/^\d+$/.test(id)) return; // a custom icon file on the clock
  const known = icons.get(id);
  if (known) {
    if (known.waiting) known.waiting.push(done);
    return;
  }
  const e = { px: null, waiting: [done] };
  icons.set(id, e);
  const finish = () => {
    const w = e.waiting;
    e.waiting = null;
    w.forEach(f => f());
  };
  const img = new Image();
  img.crossOrigin = "anonymous";
  img.onload = () => {
    try {
      const c = el("canvas");
      c.width = c.height = 8;
      const ctx = c.getContext("2d", { willReadFrequently: true });
      ctx.imageSmoothingEnabled = false;
      ctx.drawImage(img, 0, 0, 8, 8);
      const d = ctx.getImageData(0, 0, 8, 8).data;
      e.px = [];
      for (let i = 0; i < 64; i++) {
        const [r, g, b, a] = d.slice(i * 4, i * 4 + 4);
        e.px.push(a < 64 || r + g + b < 30 ? null : `rgb(${r},${g},${b})`);
      }
    } catch { /* unreadable image: draw no icon */ }
    finish();
  };
  img.onerror = finish;
  img.src = "https://developer.lametric.com/content/apps/icon_thumbs/" + id;
}
