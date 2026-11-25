// The frame's stardate: the person's local clock as one number, YYYYMMDD.hhmmss (2026-11-24 15:30:42 →
// 20261124.153042). Used top right (a clock) and for Striker's scan time. No gpui imports, so node can check it:
// bridge.check.mjs.

const two = (n) => String(n).padStart(2, "0");

export function stardate(ms) {
  const d = new Date(ms);
  return `${d.getFullYear()}${two(d.getMonth() + 1)}${two(d.getDate())}.${two(d.getHours())}${two(d.getMinutes())}${two(d.getSeconds())}`;
}
