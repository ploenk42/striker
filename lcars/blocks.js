// Markdown blocks and a block diff for the Bridge's SPEC mode. TextView takes no CSS, so a diff is drawn per
// block: the document is split into headings, paragraphs, lists and fenced code, and each block of the working
// text is "same" or "added" (with `line`, where it starts in the text); the HEAD blocks a hunk touched come in as
// "removed" just before it.
// Plain JS without gpui, so lcars/bridge.check.mjs can test it under node.

const FENCE = /^ {0,3}(`{3,}|~{3,})/;
const HEAD = /^ {0,3}#{1,6}(\s|$)/;
const ITEM = /^ {0,3}([-*+]|\d{1,9}[.)])(\s|$)/;

const lines = (s) => String(s ?? "").replace(/\r\n?/g, "\n").split("\n");

// The blocks of a text with their 1-based line ranges: { start, end, md }.
function blocksOf(s) {
  const ls = lines(s);
  const out = [];
  let cur = null;
  const close = () => { if (cur) out.push({ start: cur.start, end: cur.end, md: cur.md.join("\n") }); cur = null; };
  for (let i = 0; i < ls.length; i++) {
    const l = ls[i];
    const n = i + 1;
    const f = FENCE.exec(l);
    if (f) {
      close();
      cur = { start: n, end: n, md: [l] };
      const mark = f[1][0];
      const len = f[1].length;
      while (++i < ls.length) {
        cur.md.push(ls[i]);
        cur.end = i + 1;
        const g = /^ {0,3}(`{3,}|~{3,})\s*$/.exec(ls[i]);
        if (g && g[1][0] === mark && g[1].length >= len) break;
      }
      close();
    } else if (!l.trim()) {
      close();
    } else if (HEAD.test(l)) {
      close();
      out.push({ start: n, end: n, md: l });
    } else {
      const item = ITEM.test(l);
      if (cur && item && cur.kind !== "list") close();
      if (!cur) cur = { kind: item ? "list" : "para", start: n, end: n, md: [] };
      cur.md.push(l);
      cur.end = n;
    }
  }
  close();
  return out;
}

export function splitBlocks(text) {
  return blocksOf(text).map((b) => b.md);
}

// Does a block overlap the lines s..s+len-1? With len 0 (git -U0: lines inserted or deleted after line s),
// only a block that the change splits counts.
const touches = (b, s, len) => (len > 0 ? b.start <= s + len - 1 && s <= b.end : s >= b.start && s < b.end);

export function diffBlocks(base, text, hunks) {
  const now = blocksOf(text);
  const before = blocksOf(base);
  const hs = (Array.isArray(hunks) ? hunks : []).map((h) => ({
    os: Number(h.old_start) || 0, ol: Number(h.old_lines) || 0, ns: Number(h.new_start) || 0, nl: Number(h.new_lines) || 0,
  })).sort((a, b) => a.ns - b.ns);
  const out = [];
  const gone = new Set();
  const removed = (h) => before.forEach((b, i) => {
    if (gone.has(i) || !touches(b, h.os, h.ol)) return;
    gone.add(i);
    out.push({ kind: "removed", md: b.md });
  });
  // where a hunk sits in the working text: its first line, or between two lines for a pure deletion
  const at = (h) => (h.nl > 0 ? h.ns : h.ns + 0.5);
  let k = 0;
  for (const b of now) {
    while (k < hs.length && at(hs[k]) <= b.end) removed(hs[k++]);
    out.push({ kind: hs.some((h) => touches(b, h.ns, h.nl)) ? "added" : "same", md: b.md, line: b.start });
  }
  while (k < hs.length) removed(hs[k++]);
  return out;
}
