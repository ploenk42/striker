// The REPO tree's geometry (no gpui imports, so node can check it: bridge.check.mjs). bridge.js paints it.
// In: Striker's /repo `graph` ({cols, now, lanes, nodes, edges}), the plans' states and tasks from /repo
// `view.plans`, the space's own topic and the size to fill (layout px). Out: strokes grouped by look, node marks,
// lane labels and the NOW rule. Lane 0 is the trunk; MAIN sits left of it. A change's label (its first lane only;
// sub-lanes of parallel tasks have none) stands on its lane where the branch starts: right-aligned, ending
// LABEL_GAP before the leftmost of the lane's first node and the sources of edges coming into the lane from other
// lanes (the fork), so the curve from the fork never runs through it. Too little room for MIN_CHARS: no label.
import { ROLE, TEXT_MIN } from "./palette.js";
import { changeStatus } from "./overview.js";

export const LANE = 20; // px between lanes: a label (LABEL_H) fits between its neighbours
export const NOW_H = 18; // the NOW tag's height (TEXT_MIN text)
export const TOP = 30; // the trunk's y: room above it for the NOW tag
export const PAD = 10; // left and right of the first and last column
export const LABEL_H = 18; // a lane label's height (TEXT_MIN text), centred on its lane
export const LABEL_CHAR = 8; // px per character of a TEXT_MIN label, on the generous side (the font is condensed)
export const LABEL_MIN = 4 * LABEL_CHAR; // MAIN's spot left of the trunk
export const LABEL_MAX = 200; // labelWidth's cap
export const LABEL_GAP = 8; // black between a label and the lane it names
export const LABEL_TEXT = TEXT_MIN;
export const MIN_CHARS = 4; // a lane label with room for fewer characters is dropped
export const MAX_STEP = 80; // px between columns that hold plan work, at most
export const FUTURE_STEP = 32; // ... after NOW: every open task keeps its node, closer together
export const SMALL = 8; // px between columns that hold only plain trunk commits
export const TINY = 3; // ... beyond the KEEP newest of a long run of them
export const KEEP = 12;
export const DASH = [8, 6]; // future parts
export const DOTS = [2, 4]; // the NOW rule
export const FUTURE = 0.45; // future parts' opacity

// legacy plan states (merged, active) and OpenSpec change states; needs_you is a coordinator asking the person,
// so only it is alert; "in progress" is blue only while someone is on it (laneState), "ready" (every task ticked,
// not archived) is gold, an interrupted change (started, nobody on it) neutral
const LANE_COLOR = { merged: ROLE.done, done: ROLE.done, ready: ROLE.done, active: ROLE.working, "in progress": ROLE.working,
  needs_you: ROLE.alert, interrupted: ROLE.neutral, blocked: ROLE.neutral, planned: ROLE.neutral };
const up = (s) => String(s ?? "").toUpperCase();

// A plan's lane state. A legacy plan (docs/superpowers: active, merged, blocked, planned) keeps its own; an
// OpenSpec change follows the OVERVIEW's rule (overview.js changeStatus): "in progress" only while someone is on
// it, "ready" when every task is ticked, "interrupted" with ticked and open tasks, "planned" with no ticks.
const STATUS_LANE = { needs_you: "needs_you", done: "done", blocked: "blocked", working: "in progress", ready: "ready",
  interrupted: "interrupted", planned: "planned" };
export function laneState(p) {
  if (!p || p.legacy || p.state === "active" || p.state === "merged") return p?.state;
  return STATUS_LANE[changeStatus(p)] ?? p.state;
}

// The width for these label texts: the longest, between LABEL_MIN and LABEL_MAX.
export function labelWidth(texts) {
  const n = Math.max(0, ...(texts ?? []).map((t) => String(t ?? "").length));
  return Math.max(LABEL_MIN, Math.min(LABEL_MAX, n * LABEL_CHAR));
}

// A lane's label within `chars` characters: name, progress and suffix when all fit, else name and progress, else
// the name alone, else the name cut with "…" (the suffix goes first, so a long AFTER never hides whose lane it is).
export function fitLabel(name, suffix, chars, progress = "") {
  const head = progress ? `${name} ${progress}` : name;
  for (const t of [suffix ? `${head} · ${suffix}` : head, head, name]) if (t.length <= chars) return t;
  return `${name.slice(0, Math.max(1, chars - 1))}…`;
}

// A change's progress from its tasks: "done/total", "" without tasks.
export function progressText(plan) {
  const tasks = Array.isArray(plan?.tasks) ? plan.tasks : [];
  if (!tasks.length) return "";
  return `${tasks.filter((t) => t.state === "done").length}/${tasks.length}`;
}

export function laneColor(lane, states) {
  if (!lane || lane.index === 0) return ROLE.frame;
  return LANE_COLOR[states.get(lane.plan)] ?? ROLE.neutral;
}

export function treeLayout(graph, { width, height, plans = [], topic = null }) {
  const cols = Math.max(1, Number(graph?.cols) || 0);
  const now = Number.isFinite(Number(graph?.now)) ? Number(graph.now) : cols - 1;
  const lanes = Array.isArray(graph?.lanes) ? graph.lanes : [];
  const nodes = Array.isArray(graph?.nodes) ? graph.nodes : [];
  const edges = Array.isArray(graph?.edges) ? graph.edges : [];
  const states = new Map((plans ?? []).map((p) => [p.topic, laneState(p)]));
  const byPlan = new Map((plans ?? []).map((p) => [p.topic, p]));
  const laneOf = new Map(lanes.map((l) => [l.index, l]));

  const Y = (lane) => TOP + lane * LANE;
  const visible = (lane) => Y(lane) + LABEL_H / 2 <= height;
  const colorOf = (lane) => laneColor(laneOf.get(lane) ?? { index: lane, plan: null }, states);

  const perLane = new Map();
  for (const n of nodes) {
    if (n.lane === 0) continue;
    if (!perLane.has(n.lane)) perLane.set(n.lane, []);
    perLane.get(n.lane).push(n);
  }

  const labelW = labelWidth(["MAIN"]);
  const x0 = labelW + LABEL_GAP; // where the trunk starts
  const xs = columnXs(cols, nodes, Math.max(1, width - x0), now).map((x) => x + x0);
  const X = (c) => xs[Math.max(0, Math.min(cols - 1, c))];
  const step = cols > 1 ? Math.max(...xs.slice(1).map((x, i) => x - xs[i])) : 0;
  const nowGap = now + 1 < cols ? xs[now + 1] - xs[now] : step || SMALL;
  const nowX = X(now) + nowGap / 2;

  // strokes are grouped by look, so a whole tree paints with a handful of paths
  const groups = new Map();
  const add = (color, w, future, seg) => {
    const key = `${color}|${w}|${future}`;
    if (!groups.has(key)) groups.set(key, { color, width: w, dash: future ? DASH : null, alpha: future ? FUTURE : 1, segs: [] });
    groups.get(key).segs.push(seg);
  };

  // the trunk: solid up to NOW, dashed after it
  const end = X(cols - 1);
  add(ROLE.frame, 3, false, { kind: "line", pts: [[x0, Y(0)], [Math.min(nowX, end + PAD), Y(0)]] });
  if (nowX < end) add(ROLE.frame, 3, true, { kind: "line", pts: [[nowX, Y(0)], [end + PAD, Y(0)]] });

  const byId = new Map(nodes.map((n) => [n.id, n]));
  const linked = new Set();
  const forkX = new Map(); // lane → leftmost x of an edge coming into it from another lane
  for (const e of edges) {
    const a = byId.get(e.from), b = byId.get(e.to);
    if (!a || !b) continue;
    linked.add(`${a.id}>${b.id}`);
    if (a.lane !== b.lane) forkX.set(b.lane, Math.min(forkX.get(b.lane) ?? Infinity, X(a.col)));
    if (a.lane === 0 && b.lane === 0) continue; // the trunk line already runs there
    if (!visible(a.lane) || !visible(b.lane)) continue;
    const future = !!(e.future || a.future || b.future);
    const color = colorOf(a.lane === 0 ? b.lane : a.lane);
    const p = [X(a.col), Y(a.lane)], q = [X(b.col), Y(b.lane)];
    if (a.lane === b.lane) add(color, 2, future, { kind: "line", pts: [p, q] });
    else for (const seg of crossing(a, b, X, Y)) add(color, 2, future, seg);
  }

  // a lane runs on between its consecutive nodes even where no edge joins them (done tasks without dependencies)
  for (const [lane, list] of perLane) {
    if (!visible(lane)) continue;
    list.sort((a, b) => a.col - b.col);
    for (let i = 1; i < list.length; i++) {
      const a = list[i - 1], b = list[i];
      if (linked.has(`${a.id}>${b.id}`)) continue;
      add(colorOf(lane), 2, !!(a.future || b.future), { kind: "line", pts: [[X(a.col), Y(lane)], [X(b.col), Y(lane)]] });
    }
  }

  // node marks: done filled, working, needs_you and interrupted rings, todo a dashed ring; commits small dots on their lane
  const marks = [];
  for (const n of nodes) {
    if (!visible(n.lane)) continue;
    const x = X(n.col), y = Y(n.lane);
    if (n.kind === "task") {
      const st = n.state;
      if (st === "done") marks.push({ x, y, r: 4, fill: ROLE.done, alpha: 1 });
      else if (st === "working") marks.push({ x, y, r: 4.5, ring: ROLE.working, alpha: 1 });
      else if (st === "needs_you") marks.push({ x, y, r: 4.5, ring: ROLE.alert, alpha: 1 });
      // interrupted: started, nobody on it: a solid neutral ring (never alert)
      else if (st === "interrupted") marks.push({ x, y, r: 4.5, ring: ROLE.neutral, alpha: 1 });
      else marks.push({ x, y, r: 4, ring: ROLE.neutral, dash: true, alpha: n.future ? FUTURE + 0.15 : 1 });
    } else if (n.kind === "merge") {
      const color = colorOf(laneOfPlan(lanes, n.plan));
      marks.push(n.future ? { x, y, r: 3.5, ring: color, dash: true, alpha: FUTURE + 0.15 } : { x, y, r: 3.5, fill: color, alpha: 1 });
    } else if (n.lane !== 0) {
      marks.push({ x, y, r: 2.5, fill: colorOf(n.lane), alpha: n.future ? FUTURE : 1 });
    } else {
      // a trunk commit: a small tick across the trunk, so the history shows
      add(ROLE.frame, 1.5, !!n.future, { kind: "line", pts: [[x, y - 5], [x, y + 5]] });
    }
  }

  // MAIN left of the trunk; each change's label on its first lane where the branch starts
  const labels = [{ x: 0, y: Y(0) - LABEL_H / 2, w: labelW, text: "MAIN", color: ROLE.frame, alpha: 1, align: "left" }];
  for (const l of lanes) {
    const list = perLane.get(l.index);
    if (l.index === 0 || !l.plan || !l.label || !visible(l.index) || !list?.length) continue;
    const first = Math.min(...list.map((n) => X(n.col)));
    const right = Math.min(first, forkX.get(l.index) ?? Infinity) - LABEL_GAP;
    const chars = Math.floor(right / LABEL_CHAR);
    if (chars < MIN_CHARS) continue;
    const extra = [];
    if (topic && l.plan === topic) extra.push("THIS PLAN");
    const deps = (l.blocked_by ?? []).filter(Boolean);
    if (deps.length) extra.push(`AFTER ${deps.map(up).join(", ")}`);
    const text = fitLabel(up(l.plan), extra.join(" · "), chars, progressText(byPlan.get(l.plan)));
    const w = Math.min(right, text.length * LABEL_CHAR);
    labels.push({ x: right - w, y: Y(l.index) - LABEL_H / 2, w, text, color: colorOf(l.index), align: "right",
      alpha: list.every((n) => n.future) ? 0.75 : 1 });
  }

  // lanes below the field's bottom are cut off: say how many
  const hidden = lanes.filter((l) => l.index !== 0 && !visible(l.index)).length;
  return { step, xs, nowX, x0, labelW, strokes: [...groups.values()], marks, labels, hidden };
}

// Each column's x. A column with plan work (a task, a merge, anything off the trunk) gets the full step; a column
// of plain trunk commits a SMALL one, and in a run of more than KEEP of them the older ones only TINY. The gap
// An edge between two lanes, as git graphs draw it: over one column a single curve; over more, toward the trunk
// (a merge) it runs on its own lane and bends in the last gap, away from it (a fork) it bends in the first gap and
// runs on in its new lane. So it never cuts across the lanes and nodes between.
export function crossing(a, b, X, Y) {
  const p = [X(a.col), Y(a.lane)], q = [X(b.col), Y(b.lane)];
  if (b.col - a.col <= 1) return [{ kind: "curve", pts: [p, q] }];
  if (b.lane < a.lane) {
    const bend = [X(b.col - 1), Y(a.lane)];
    return [{ kind: "line", pts: [p, bend] }, { kind: "curve", pts: [bend, q] }];
  }
  const bend = [X(a.col + 1), Y(b.lane)];
  return [{ kind: "curve", pts: [p, bend] }, { kind: "line", pts: [bend, q] }];
}

// into a column is wide when either side holds plan work, so curves have room. Wide gaps after the one following
// NOW (that one holds the NOW tag) are at most FUTURE_STEP. Too many plan columns for the width: every gap
// shrinks alike.
export function columnXs(cols, nodes, width, now = cols - 1) {
  const rich = new Array(cols).fill(false);
  for (const n of nodes) {
    const c = Number(n.col);
    if (c >= 0 && c < cols && (n.kind !== "commit" || n.lane !== 0)) rich[c] = true;
  }
  const gaps = new Array(cols).fill(0); // gaps[c]: from column c-1 to c
  const wide = new Array(cols).fill(false);
  for (let c = 1; c < cols; c++) wide[c] = rich[c] || rich[c - 1];
  // plain runs: the KEEP newest gaps SMALL, older ones TINY
  for (let c = cols - 1; c >= 1;) {
    if (wide[c]) { c--; continue; }
    let s = c;
    while (s - 1 >= 1 && !wide[s - 1]) s--;
    for (let k = c, i = 0; k >= s; k--, i++) gaps[k] = i < KEEP ? SMALL : TINY;
    c = s - 1;
  }
  const fut = (c) => c >= now + 2;
  const avail = Math.max(1, width - 2 * PAD);
  const plain = gaps.reduce((a, b) => a + b, 0);
  let nPast = 0, nFut = 0;
  for (let c = 1; c < cols; c++) if (wide[c]) fut(c) ? nFut++ : nPast++;
  let fstep = nFut ? Math.min(FUTURE_STEP, (avail - plain) / nFut) : 0;
  let full = nPast ? Math.min(MAX_STEP, (avail - plain - nFut * fstep) / nPast) : 0;
  if (nPast && nFut && full < fstep) full = fstep = (avail - plain) / (nPast + nFut); // share alike
  let scale = 1;
  if (nPast + nFut && Math.min(nPast ? full : Infinity, nFut ? fstep : Infinity) < SMALL) {
    full = fstep = SMALL;
    scale = avail / (plain + (nPast + nFut) * SMALL);
  }
  if (!(nPast + nFut) && plain > avail) scale = avail / plain;
  const xs = [PAD];
  for (let c = 1; c < cols; c++) xs.push(xs[c - 1] + (wide[c] ? (fut(c) ? fstep : full) : gaps[c]) * scale);
  return xs;
}

function laneOfPlan(lanes, plan) {
  return lanes.find((l) => l.index !== 0 && l.plan === plan)?.index ?? 0;
}
