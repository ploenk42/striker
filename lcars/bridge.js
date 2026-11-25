// The Bridge for the LCARS frame: one view per space (= worktree). OVERVIEW mode (the default on a main checkout)
// shows the repo: Vision, its changes, Register and Done on the elbow, the coordinators as cards. PLAN mode shows the worktree's plan as an
// LCARS elbow with one row per task, and three status columns (WORKING, NEEDS YOU, DONE) whose cards sit on
// their task's row. SPEC mode shows the plan's spec: a narrower elbow listing its `##` headings, and a reading
// area with the spec's markdown, changes against HEAD marked per block (blocks.js). Under it the bottom field:
// a rule, and the REPO tree (tree.js) or a live terminal (the coordinator's, or a WORKING card's agent). The data comes from
// `striker serve` (127.0.0.1:7522/space, /repo, /doc and /overview); this file only draws.
// Answers are kept in shared state, keyed by the space's place, and written only when they changed.
import { div, PathBuilder } from "gpui-kit";
import { v_flex, h_flex, TextView } from "gpui-base";
import { agents, exec, layout, panes, run, settings, setShared, shared, spaces, terminals } from "herdr";
import { ROLE, TEXT_MIN, TEXT_BODY, TOUCH } from "./palette.js";
import { VIEW_KEY, FLEET_KEY } from "./striker.js";
import { diffBlocks } from "./blocks.js";
import { treeLayout, DOTS, LABEL_H, LABEL_TEXT, NOW_H } from "./tree.js";
import { terminalState, cardBottom, ruleColor } from "./addr.js";
import { repoPathFor, spaceTerminals, repoNotice } from "./space.js";
import { overviewRows, coordCard, changeColor, changeStatus, interruptedText, nowTask, afterText, briefingLine, itemLabel,
  registerSummary, isMainCheckout, defaultMode, changeSpace, repoName, docHeadings, newestFirst, doneSummary, defaultOpen, tocLine,
  barColors, firstMode, commitsLine, planCard, READY_TEXT, specList } from "./overview.js";

export const BRIDGE_KEY = "bridge.view"; // "overview" | "plan" | "spec" while the Bridge is shown, else null
export const OVERVIEW_KEY = "bridge.overview"; // the /overview answer for the shown place
// what OVERVIEW picked for PLAN / SPEC in this place: { machine, cwd, change, file, title } — a change without a
// worktree (read from the main checkout, read-only) or the vision file for SPEC; null: the branch's own plan
export const PICK_KEY = "bridge.pick";
export const OV_OPEN_KEY = "bridge.ovopen"; // OVERVIEW's expanded row: { machine, cwd, key } (key null: all collapsed)
// the open change row's table of contents: { machine, cwd, change, parts: [{ label, heads }] } (from /doc, so each
// heading keeps its file's label; /repo's view.plans[].toc has the same headings flat and is the fallback)
export const TOC_KEY = "bridge.toc";
export const CHOSEN_KEY = "bridge.chosen"; // { mode } while the shown mode is the person's choice, else null
export const SPACE_KEY = "bridge.space"; // the /space answer for the shown place
export const REPO_KEY = "bridge.repo"; // the /repo answer for the shown place
export const DOC_KEY = "bridge.doc"; // the /doc answer for the plan's spec (SPEC mode), with its `file`
export const AT_KEY = "bridge.at"; // SPEC mode's heading last clicked: { file, i }
// SPEC on a main checkout: the spec picked from the list of all specs, { machine, cwd, file } (none: the first)
export const MAINSPEC_KEY = "bridge.mainspec";
export const OPEN_KEY = "bridge.open"; // the expanded task: { topic, n } (n null: all collapsed)
export const BOTTOM_KEY = "bridge.bottom"; // { kind: "repo" } | { kind: "terminal", address, label }
export const TICK = 2000; // how often the poller looks whether an answer is due (cheap: no fetch, no redraw)
const SPACE_POLL = 10000;
const REPO_POLL = 60000;
const DOC_POLL = 10000;
const OVERVIEW_POLL = 10000;
const TOC_POLL = 60000;
const BASE = "http://127.0.0.1:7522";

// sizes: no text under TEXT_MIN, rows TEXT_BODY, every clickable thing at least TOUCH high (palette.js)
const TITLE = 22; // the arm's title
const HEAD = 18; // the column headings' words
const GAP = 10; // black between the elbow and the columns, and between columns
const ARM = 52; // the height of the elbow's arm (clickable: the coordinator) and of the column headings
const OUTER = 44; // the elbow's outer top-left radius
const INNER = 24; // the elbow's concave inner corner radius
const NECK = 30; // the width (and height) of the concave corner piece
const ROW = 60; // a closed task row (≥ TOUCH; its card holds a title, a status line and the step bar)
const ROW_OV = 64; // a closed OVERVIEW row: its card holds three lines and the tick bar
const ARM_OV = 84; // OVERVIEW: the arm carries the repo name, the briefing line and the last commits; the column headings match it
const LEFT = "52%"; // the elbow's share of the width, arm included
const LINE_IN = `${ROLE.ink}33`; // row lines inside the elbow: ink at 20 % over the frame
const SPEC_W = 280; // SPEC mode: the width of the elbow's vertical part

const up = (s) => String(s ?? "").toUpperCase();
const press = (el, fn) => el.cursor_pointer().hover((s) => s.opacity(0.85)).on_click(() => fn());

// --- where -----------------------------------------------------------------------------------------

// Where the shown space's repo is (space.js): its fleet entry (Striker's /status, polled by the sidebar), else a
// coordinator's worktree, else a local lead's folder, else where the space was made.
function where() {
  const s = spaces().find((x) => x.active);
  if (!s) return null;
  const terms = spaceTerminals(s, panes(), terminals());
  const leads = s.machine ? [] : terms.filter((t) => t.labels["striker.lead"]);
  const r = repoPathFor(s, shared(FLEET_KEY)?.repos ?? [], leads, terms);
  return r ? { machine: r.machine ?? null, cwd: r.path, branch: s.branch ?? null } : null;
}
const samePlace = (v, w) => !!v && !!w && (v.machine ?? null) === w.machine && v.cwd === w.cwd;

// a terminal may sit in a subfolder: Striker wants the worktree's top folder
const tops = new Map();
async function toplevel(machine, cwd) {
  const key = `${machine ?? ""}|${cwd}`;
  if (tops.has(key)) return tops.get(key);
  let top = cwd;
  try {
    const r = await exec("git", ["rev-parse", "--show-toplevel"], { cwd, on: machine, timeout: 10000 });
    if (r.code === 0 && r.stdout.trim()) top = r.stdout.trim();
  } catch { /* not git, or unreachable: ask with the folder as it is */ }
  tops.set(key, top);
  return top;
}

// whether the worktree at `path` is the repo's main checkout (OVERVIEW is its default mode); a failed ask is not kept
const mains = new Map();
async function mainCheckout(machine, path) {
  const key = `${machine ?? ""}|${path}`;
  if (mains.has(key)) return mains.get(key);
  try {
    const r = await exec("git", ["rev-parse", "--git-common-dir"], { cwd: path, on: machine, timeout: 10000 });
    if (r.code !== 0) return false;
    const main = isMainCheckout(path, r.stdout);
    mains.set(key, main);
    return main;
  } catch { return false; }
}

// OVERVIEW's pick for this place (PICK_KEY), and a change's plan from /repo's view (read from the main checkout)
const pickFor = (w) => { const p = shared(PICK_KEY); return samePlace(p, w) ? p : null; };
function viewPlan(w, topic) {
  const r = shared(REPO_KEY);
  if (!samePlace(r, w)) return null;
  return (r.data?.view?.plans ?? []).find((p) => p.topic === topic) ?? null;
}

// the /overview answer for this place, its rows, and the row that is open (the person's pick, else the default)
function overviewHere(w) {
  const o = shared(OVERVIEW_KEY);
  return samePlace(o, w) ? o : null;
}
// PLAN and SPEC on a main checkout (no plan of its own, nothing picked in OVERVIEW): PLAN reads the vision file,
// SPEC one spec of the repo's list (the picked one, else the first). null until /overview answered, or nothing to read.
function mainDoc(w, mode) {
  const ov = overviewHere(w)?.data;
  if (!ov) return null;
  if (mode === "plan") return ov.context?.file ? { file: ov.context.file, title: ov.context.title || ov.context.file } : null;
  if (mode !== "spec") return null;
  const list = specList(ov);
  const m = shared(MAINSPEC_KEY);
  const s = (samePlace(m, w) ? list.find((x) => x.file === m.file) : null) ?? list[0];
  return s ? { file: s.file, title: s.label, list } : null;
}
// whether the shown place is a checkout without a plan of its own and nothing picked (PLAN and SPEC use mainDoc)
function onMain(w) {
  const st = shared(SPACE_KEY);
  return !pickFor(w) && samePlace(st, w) && !!st.data && !st.data.plan;
}

function openRow(w, rows) {
  const o = shared(OV_OPEN_KEY);
  return samePlace(o, w) ? o.key ?? null : defaultOpen(rows);
}

// The files whose `##` headings make a change's table of contents: proposal.md and design.md next to its spec
// (an OpenSpec change), else the spec itself (a legacy plan).
function tocFiles(w, name) {
  const spec = viewPlan(w, name)?.spec ?? "";
  const dir = spec.includes("/") ? spec.replace(/\/[^/]*$/, "") : `openspec/changes/${name}`;
  if (!/(^|\/)openspec\/changes\//.test(`${dir}/`)) return spec ? [["spec", spec]] : [];
  return [["proposal", `${dir}/proposal.md`], ["design", `${dir}/design.md`]];
}

async function askToc(w, q, name) {
  const parts = [];
  for (const [label, file] of tocFiles(w, name)) {
    let heads = [];
    try {
      const r = await fetch(`${BASE}/doc?${q}&file=${encodeURIComponent(file)}`);
      const body = r.ok ? await r.json() : null;
      if (body && !body.error) heads = docHeadings(body.text);
    } catch { /* not running or unreadable: no headings */ }
    parts.push({ label, heads });
  }
  const next = { machine: w.machine, cwd: w.cwd, change: name, parts };
  if (JSON.stringify(next) !== JSON.stringify(shared(TOC_KEY) ?? null)) setShared(TOC_KEY, next);
}

// --- data ------------------------------------------------------------------------------------------

let asking = 0; // when the running poll started; one at a time across this mod's copies
const due = { space: { key: "", at: 0 }, repo: { key: "", at: 0 }, doc: { key: "", at: 0 }, overview: { key: "", at: 0 },
  toc: { key: "", at: 0 } };
let lastMode = null; // the mode the last poll saw: SPEC asks /doc at once when it opens
let lastPlace = null; // the place the last poll saw while the Bridge was shown

// Called every TICK by the Bridge's copy: does nothing while the Bridge is hidden; asks /space every 10 s and
// /repo every 60 s, and at once when the shown place changed; in OVERVIEW mode also /overview every 10 s; in SPEC
// mode also /doc for the plan's spec (or OVERVIEW's pick), at once when SPEC opens and every 10 s while it is open.
// When the place changes under a shown Bridge, its mode goes to the place's default: OVERVIEW on a main checkout,
// PLAN in a linked worktree; OVERVIEW's pick is dropped. On the first poll of a shown Bridge the same default
// applies on a main checkout, unless the person chose the mode (CHOSEN_KEY, set by a pseudo-tab or command).
export async function pollBridge() {
  let mode = shared(BRIDGE_KEY);
  if (!mode) { lastMode = null; lastPlace = null; return; }
  const w = where();
  if (!w) return;
  if (asking && Date.now() - asking < 30000) return;
  asking = Date.now();
  try {
    const path = await toplevel(w.machine, w.cwd);
    const key = `${w.machine ?? ""}|${w.cwd}`;
    if (lastPlace !== null && lastPlace !== key) {
      const want = defaultMode(await mainCheckout(w.machine, path));
      if (shared(PICK_KEY)) setShared(PICK_KEY, null);
      if (shared(CHOSEN_KEY)) setShared(CHOSEN_KEY, null);
      if (shared(BRIDGE_KEY) === mode && want !== mode) { setShared(BRIDGE_KEY, want); mode = want; }
    } else if (lastPlace === null) {
      const want = firstMode(mode, await mainCheckout(w.machine, path), !!shared(CHOSEN_KEY));
      if (shared(BRIDGE_KEY) === mode && want !== mode) { setShared(BRIDGE_KEY, want); mode = want; }
    }
    lastPlace = key;
    const q = `machine=${encodeURIComponent(w.machine ?? "")}&path=${encodeURIComponent(path)}`;
    const now = Date.now();
    if (due.space.key !== key || now - due.space.at >= SPACE_POLL) {
      due.space = { key, at: now };
      await ask(SPACE_KEY, `${BASE}/space?${q}`, w, path);
    }
    if (due.repo.key !== key || now - due.repo.at >= REPO_POLL) {
      due.repo = { key, at: now };
      await ask(REPO_KEY, `${BASE}/repo?${q}`, w, path);
    }
    // /overview: in OVERVIEW, and for PLAN (vision) and SPEC (list of specs) on a checkout without a plan
    const main = onMain(w);
    if ((mode === "overview" || main) && (lastMode !== mode || due.overview.key !== key || now - due.overview.at >= OVERVIEW_POLL)) {
      due.overview = { key, at: now };
      await ask(OVERVIEW_KEY, `${BASE}/overview?${q}`, w, path);
    }
    // the open change row's table of contents: at once when another row opens, else every 60 s
    if (mode === "overview") {
      const ov = overviewHere(w);
      const open = ov?.data ? openRow(w, overviewRows(ov.data)) : null;
      if (typeof open === "string" && open.startsWith("change:")) {
        const name = open.slice("change:".length);
        const tkey = `${key}|${name}`;
        if (due.toc.key !== tkey || now - due.toc.at >= TOC_POLL) {
          due.toc = { key: tkey, at: now };
          await askToc(w, q, name);
        }
      }
    }
    const st = shared(SPACE_KEY);
    const pick = pickFor(w);
    const spec = pick ? pick.file || (pick.change ? viewPlan(w, pick.change)?.spec : null) || null
      : samePlace(st, w) ? st.data?.plan?.spec ?? (main ? mainDoc(w, mode)?.file : null) : null;
    if ((mode === "spec" || (mode === "plan" && main)) && spec) {
      const dkey = `${key}|${spec}`;
      if (lastMode !== mode || due.doc.key !== dkey || now - due.doc.at >= DOC_POLL) {
        due.doc = { key: dkey, at: now };
        await ask(DOC_KEY, `${BASE}/doc?${q}&file=${encodeURIComponent(spec)}`, w, path, { file: spec });
      }
    }
    lastMode = mode;
  } finally {
    asking = 0;
  }
}

// One endpoint into shared state: { data, error, machine, cwd, path }. An error keeps the last good data
// of the same place (and file), so the view can say "unreadable" over what it showed before.
async function ask(key, url, w, path, extra = {}) {
  const prev = shared(key);
  const mine = samePlace(prev, w) && prev.file === extra.file ? { data: prev.data ?? null } : { data: null };
  let next;
  try {
    const r = await fetch(url);
    let body = null;
    try { body = await r.json(); } catch { /* unparsable body, handled below */ }
    if (!r.ok) next = { ...mine, error: `STRIKER SERVE ANSWERED ${r.status}` };
    else if (!body || typeof body !== "object") next = { ...mine, error: "STRIKER SERVE BAD ANSWER" };
    else if (body.error) next = { ...mine, error: String(body.error) };
    else next = { data: body, error: null };
  } catch {
    next = { ...mine, error: "STRIKER SERVE NOT RUNNING" };
  }
  next = { ...next, machine: w.machine, cwd: w.cwd, path, ...extra };
  if (JSON.stringify(next) !== JSON.stringify(prev ?? null)) setShared(key, next);
}

// --- actions ---------------------------------------------------------------------------------------

// The Bridge and Striker's board share the work area: showing the Bridge hides the board. Back on OVERVIEW, PLAN
// and SPEC show the branch's own plan again; PLAN drops a pick that is only a file (the vision for SPEC).
export function showBridge(mode) {
  if (!mode) restoreBottom();
  const pick = shared(PICK_KEY);
  if (pick && (mode === "overview" || (mode === "plan" && !pick.change))) setShared(PICK_KEY, null);
  // a mode shown here is the person's choice (pseudo-tab, command, a click in OVERVIEW): the first poll keeps it
  setShared(CHOSEN_KEY, mode ? { mode } : null);
  setShared(BRIDGE_KEY, mode || null);
  if (mode) setShared(VIEW_KEY, null);
  run("layout.select", { stack: "work-view", child: mode ? "bridge" : "main" });
}

// The bottom field is the layout block `bridge-bottom` under the rule block `bridge-rule`. Showing a terminal
// rewrites that block's content in the shown layout (layout.set) to { terminal }, and back to this mod for the
// tree; nothing else in the tree changes, so swapping back leaves the layout file as it was.
const BOTTOM_ID = "bridge-bottom";
const BOTTOM_MOD = { mod: "lcars", props: { part: "bridge-bottom" } };
const FIELD_ID = "bridge-field";
const RULE_ID = "bridge-rule";
const STACK_ID = "work-view";
const STACK_DEFAULT = "main";
const same = (a, b) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);

function findNode(node, id) {
  if (!node || typeof node !== "object") return null;
  if (node.id === id) return node;
  for (const c of Array.isArray(node.children) ? node.children : []) {
    const hit = findNode(c, id);
    if (hit) return hit;
  }
  return null;
}

// what the bottom block holds in the shown layout now (null: the layout has no such block)
function bottomContent() {
  return findNode(layout(), BOTTOM_ID)?.content ?? null;
}

// When a terminal block's terminal ends, Herdr drops the whole block from the layout (and the file): then the
// block is put back under the rule, as the repo layout has it.
function swapBottom(content) {
  const tree = layout();
  if (!findNode(tree, BOTTOM_ID) && !findNode(tree, FIELD_ID)) return false;
  const next = JSON.parse(JSON.stringify(tree));
  let node = findNode(next, BOTTOM_ID);
  if (!node) {
    const field = findNode(next, FIELD_ID);
    if (!Array.isArray(field.children)) return false;
    node = { id: BOTTOM_ID, type: "block", content, size: { weight: 1 } };
    const rule = field.children.findIndex((c) => c?.id === RULE_ID);
    field.children.splice(rule + 1, 0, node);
  } else if (same(node.content, content)) return true;
  node.content = content;
  // layout() carries the stack's shown child, and layout.set would write it into the file: the file keeps the
  // shipped "main", and the shown child is selected again right after (layout.select does not write the file)
  const stack = findNode(next, STACK_ID);
  const shown = stack?.active ?? null;
  if (stack) stack.active = STACK_DEFAULT;
  // repo layouts/lcars.json is kept in Herdr's own key order/LF so a swap back is byte-identical
  run("layout.set", { layout: next });
  if (shown && shown !== STACK_DEFAULT) run("layout.select", { stack: STACK_ID, child: shown });
  return true;
}

// What the bottom field shows: { kind: "repo" } or { kind: "terminal", address, label }.
export function setBottom(next) {
  const b = next?.kind === "terminal" && next.address ? { machine: where()?.machine ?? null, ...next } : { kind: "repo" };
  setShared(BOTTOM_KEY, b);
  swapBottom(b.kind === "terminal" ? { terminal: b.address } : BOTTOM_MOD);
}

// Back to the tree, also in the layout file, whenever the Bridge goes away or the shown terminal is gone.
export function restoreBottom() {
  if (shared(BOTTOM_KEY)?.kind === "terminal") setShared(BOTTOM_KEY, { kind: "repo" });
  if (!same(bottomContent(), BOTTOM_MOD)) swapBottom(BOTTOM_MOD);
}

// Called every TICK by the rule's copy: the layout file must not keep a terminal the Bridge no longer means to
// show (after an app restart, when the terminal ended, or when the Bridge was left another way).
export function checkBottom() {
  const c = bottomContent();
  if (!c) {
    // the shown terminal ended and Herdr dropped its block: the tree comes back
    if (findNode(layout(), FIELD_ID)) restoreBottom();
    return;
  }
  const b = shared(BOTTOM_KEY);
  if (b?.kind === "terminal" && shared(BRIDGE_KEY)) {
    // only a terminal positively known gone; a remote one may be listed as "machine/tN" while Striker said "tN"
    if (terminalState(b.address, terminals(), b.machine ?? null) === "gone") restoreBottom();
    return;
  }
  if (b?.kind === "terminal") setShared(BOTTOM_KEY, { kind: "repo" });
  if (!same(c, BOTTOM_MOD)) swapBottom(BOTTOM_MOD);
}

// lcars.plan:<change> (herdr-app do): PLAN of that change read from this place, as OVERVIEW's PLAN ▸ shows a change
// without a worktree
export function pickChange(name) {
  const w = where();
  if (!w || !name) return;
  setShared(PICK_KEY, { machine: w.machine, cwd: w.cwd, change: name, title: name });
  showBridge("plan");
}

// lcars.coordinator / lcars.repo (keys.json, herdr-app do): what a click on the plan title or ◂ REPO does
export function bottomCommand(c) {
  if (c === "repo") { setBottom({ kind: "repo" }); return; }
  if (c !== "coordinator") return;
  const plan = shared(SPACE_KEY)?.data?.plan;
  const coord = plan?.coordinator || shared(SPACE_KEY)?.data?.coordinator || null;
  if (plan && coord) setBottom({ kind: "terminal", address: coord, label: `COORDINATOR · ${up(plan.title || plan.topic)}` });
}

// --- drawing ---------------------------------------------------------------------------------------

// an open task's oval is the row's own orange, so only its ink outline shows
const OVAL = { todo: ROLE.frame, interrupted: ROLE.neutral, working: ROLE.working, needs_you: ROLE.alert, done: ROLE.done };
const COLUMNS = [["working", "WORKING", ROLE.working], ["needs_you", "NEEDS YOU", ROLE.alert], ["done", "DONE", ROLE.done]];
const short = (t) => (t.issue != null ? `#${t.issue}` : `TASK ${t.id || t.n}`);

// The task's steps: its top-level "- [ ]" / "- [x]" lines, each with the indented lines under it.
function steps(body) {
  const out = [];
  let cur = null;
  for (const line of String(body ?? "").split(/\r?\n/)) {
    const m = /^- \[( |x|X)\]/.exec(line);
    if (m) { cur = { done: m[1] !== " ", md: line }; out.push(cur); }
    else if (cur && /^\s+\S/.test(line)) cur.md += `\n${line}`;
    else if (line.trim()) cur = null;
  }
  return out;
}

// the task the person is most likely on: working, else waiting on them, else interrupted, else the first not started
const current = (tasks) => nowTask(tasks)?.n ?? null;

function oval(t) {
  return div().w(24).h(14).flex_none().rounded(7).bg(OVAL[t.state] ?? ROLE.frame).border(t.state === "todo" || !OVAL[t.state] ? 2 : 1)
    .border_color(ROLE.ink)
    .when(t.state === "needs_you", (el) => el.loop("opacity", { from: 0.4, to: 1, period: 700 }));
}

function stepList(t) {
  const list = steps(t.body);
  if (!list.length) return div().px(12).py(6).text_size(TEXT_MIN).font_weight(700).text_color(ROLE.ink).child("NO STEPS");
  const at = t.state === "done" ? -1 : list.findIndex((s) => !s.done);
  return v_flex().w_full().px(12).pt(4).pb(8).gap(2).children(list.map((s, j) => {
    const lines = plain(s.md);
    if (j === at) lines[0] += "  ◂ NOW";
    return v_flex().id(`bridge-step-${t.n}-${j}`).w_full().px(6).py(1).text_size(TEXT_BODY)
      .when(j === at, (el) => el.bg(ROLE.bg).text_color(ROLE.active).font_weight(700).rounded(4))
      .when(j !== at, (el) => el.text_color(ROLE.ink))
      .children(lines.map((l, k) => div().w_full().when(k > 0, (e) => e.pl(18)).child(l)));
  }));
}

// TextView.markdown draws in the theme's foreground whatever text_color says, which cannot be read on orange,
// so a step is set as plain text: checkbox glyph, emphasis and code marks dropped, sub-items one line each.
function plain(md) {
  return md.split("\n").map((l, i) => {
    let s = l.replace(/\*\*|__|`/g, "");
    if (i === 0) return s.replace(/^- \[ \]\s*/, "☐ ").replace(/^- \[[xX]\]\s*/, "☑ ");
    return s.trim().replace(/^[-*]\s+/, "· ");
  });
}

// a step/tick bar; `color` paints the done segments (and the next one unless it is `working`: barColors)
function stepBar(t, color = ROLE.working) {
  const total = Math.max(0, Math.min(24, Number(t.steps_total) || 0));
  const done = Math.max(0, Math.min(total, Number(t.steps_done) || 0));
  if (!total) return null;
  return h_flex().w_full().h(4).flex_none().gap(2).mt(3).children(barColors(done, total, color).map((c) =>
    div().flex_1().h_full().bg(c)));
}

// A task's card (overview.js planCard): a status strip on the left and its text on black — WORKING, NEEDS YOU,
// INTERRUPTED (neutral), DONE (gold) — or OPEN, a neutral outline without fill. A WORKING or NEEDS YOU card with an
// agent is clickable, except in a read-only plan.
function card(t, live, readOnly) {
  const pc = planCard(t);
  const color = pc.color;
  const agent = (t.agents ?? [])[0];
  const tab = agent ? up(live.get(agent.address)?.tab_name ?? agent.role ?? agent.address) : "NO AGENT";
  const step = Math.min((Number(t.steps_done) || 0) + 1, Number(t.steps_total) || 0);
  const bar = pc.kind === "working" ? stepBar(t) : null;
  const sub = { working: `${tab} · STEP ${step}/${t.steps_total}`, needs_you: `${tab} · NEEDS YOU`, interrupted: "INTERRUPTED · NOBODY ON IT",
    open: "OPEN", done: "DONE" }[pc.kind];
  const text = pc.outline ? ROLE.neutral : ROLE.text;
  const el = h_flex().id(`bridge-card-${t.n}`).size_full().gap(8).items_stretch()
    .when(pc.outline, (e) => e.border(2).border_color(color).pl(8))
    .when(!pc.outline, (e) => e.child(div().w(8).flex_none().bg(color)
      .when(pc.kind === "needs_you", (e) => e.loop("opacity", { from: 0.4, to: 1, period: 700 }))))
    .child(v_flex().flex_1().min_w(0).justify_center().py(3)
      .child(div().text_size(TEXT_BODY).font_weight(700).text_color(text).truncate().child(up(`${short(t)} ${t.title}`)))
      .child(div().text_size(TEXT_MIN).font_weight(700).text_color(color).truncate().child(sub))
      .when(!!bar, (e) => e.child(bar)));
  const target = readOnly ? null : cardBottom(t.state, agent, `${short(t)} · ${tab} · LIVE`);
  return target ? press(el, () => setBottom(target)) : el;
}

// the elbow's part of one row: the frame cell and the black gutter under the concave corner
const elbowCell = (child) => h_flex().w(LEFT).flex_none().items_stretch()
  .child(child)
  .child(div().w(NECK).flex_none());

// every task has its card on its row; a read-only plan (a change without a worktree, read from the main
// checkout) shows the same cards, none clickable. The card is as tall as the row.
function row(t, openN, live, readOnly = false) {
  const open = openN === t.n;
  const head = h_flex().id(`bridge-task-${t.n}`).w_full().h(ROW).flex_none().gap(10).items_center().px(12)
    .bg(open ? ROLE.bg : ROLE.frame).text_color(open ? ROLE.active : ROLE.ink).cursor_pointer()
    .on_click(() => setShared(OPEN_KEY, { topic: t.topic, n: open ? null : t.n }))
    .child(oval(t))
    .child(div().flex_1().min_w(0).truncate().text_size(TEXT_BODY).font_weight(700).child(up(`${short(t)} · ${t.title}`)))
    .child(div().flex_none().text_size(TEXT_BODY).font_weight(700).child(open ? "▾" : "▸"));
  const frame = v_flex().flex_1().min_w(0).bg(ROLE.frame).child(head).when(open, (el) => el.child(stepList(t)));
  const col = planCard(t).col;
  return h_flex().id(`bridge-row-${t.n}`).w_full().flex_none().items_stretch()
    .child(elbowCell(frame))
    .child(div().w(GAP).flex_none())
    .child(h_flex().flex_1().min_w(0).gap(GAP).items_stretch().children(COLUMNS.map(([st]) =>
      v_flex().flex_1().min_w(0).child(col === st ? card(t, live, readOnly) : div()))));
}

// rows are separated by 2 px lines: darker orange inside the elbow, dark gray under the columns
const line = (i) => h_flex().id(`bridge-line-${i}`).w_full().h(2).flex_none()
  .child(elbowCell(div().flex_1().h_full().bg(ROLE.frame).child(div().size_full().bg(LINE_IN))))
  .child(div().w(GAP).flex_none())
  .child(div().flex_1().h_full().bg(ROLE.track));

// the three square column headings
const columnHeads = () => h_flex().flex_1().min_w(0).h_full().gap(GAP).children(COLUMNS.map(([st, word, color]) =>
  div().id(`bridge-col-${st}`).flex_1().min_w(0).h_full().bg(color).flex().items_end().justify_end().px(10).pb(4)
    .text_color(ROLE.ink).text_size(HEAD).font_weight(700).child(word)));

// the elbow's arm with a title (clickable when it does something), and the three square column headings
function header(title, onTitle) {
  const arm = div().id("bridge-title").w(LEFT).h_full().flex_none().bg(ROLE.frame).rounded_tl(OUTER)
    .flex().items_center().pl(OUTER).pr(12).text_color(ROLE.ink).text_size(TITLE).font_weight(700)
    .child(div().min_w(0).truncate().child(up(title)));
  return h_flex().w_full().h(ARM).flex_none().gap(GAP)
    .child(onTitle ? press(arm, onTitle) : arm)
    .child(columnHeads());
}

// under the arm: the elbow goes on down, and the concave corner turns into the black of the rows
const neck = () => h_flex().w_full().h(NECK).flex_none()
  .child(h_flex().w(LEFT).flex_none().h_full()
    .child(div().flex_1().h_full().bg(ROLE.frame))
    .child(div().w(NECK).h_full().flex_none().bg(ROLE.frame).child(div().size_full().bg(ROLE.bg).rounded_tl(INNER))));

// the elbow's foot: it runs down to the bottom and ends in a rounded corner
const foot = () => h_flex().id("bridge-foot").w_full().flex_1().min_h(INNER).items_stretch()
  .child(elbowCell(div().flex_1().bg(ROLE.frame).rounded_bl(INNER)));

// a whole-view message (no plan, unreadable, scanning): the elbow's arm carries it
function notice(text, color, sub) {
  return v_flex().id("bridge").size_full().bg(ROLE.bg).gap(10)
    .child(div().id("bridge-notice").w_full().h(ARM).flex_none().bg(color).rounded_tl(OUTER).flex().items_center()
      .pl(OUTER).pr(12).text_color(ROLE.ink).text_size(TITLE).font_weight(700).child(div().truncate().child(up(text))))
    .when(!!sub, (el) => el.child(div().pl(OUTER).text_size(TEXT_MIN).font_weight(700).text_color(ROLE.alert).child(up(sub))));
}

// --- SPEC mode -------------------------------------------------------------------------------------

const H2 = /^ {0,3}##\s+/;
const headingText = (md) => md.replace(H2, "").replace(/\s+#+\s*$/, "").trim();

// The reading area's direct children: runs of unchanged blocks joined into one markdown text, each changed block
// on its own, and a new child at every `##` heading so a heading click can scroll to it. Hunks are counted per
// section (the section holding the hunk's first line) for the elbow's `+a −r`.
function specParts(doc) {
  const blocks = diffBlocks(doc.base, doc.text, doc.hunks);
  const parts = [];
  const heads = []; // per section: { title, child, line, a, r }
  for (const b of blocks) {
    const h2 = b.kind !== "removed" && H2.test(b.md);
    if (h2) heads.push({ title: headingText(b.md), child: parts.length, line: b.line, a: 0, r: 0 });
    const last = parts[parts.length - 1];
    if (b.kind === "same" && !h2 && last?.kind === "same") last.md += `\n\n${b.md}`;
    else parts.push({ kind: b.kind, md: b.md });
  }
  let a = 0, r = 0;
  for (const h of Array.isArray(doc.hunks) ? doc.hunks : []) {
    const add = Number(h.new_lines) || 0, rem = Number(h.old_lines) || 0;
    a += add; r += rem;
    let s = null;
    for (const x of heads) if (x.line <= (Number(h.new_start) || 0)) s = x;
    if (s) { s.a += add; s.r += rem; }
  }
  return { parts, heads, a, r };
}

// a removed block is plain text, its markdown marks dropped, struck through in neutral: a code block keeps its
// lines, a list one line per item, anything else wraps as one paragraph
function struck(md) {
  const ls = md.split("\n");
  if (/^ {0,3}(```|~~~)/.test(ls[0])) return ls.map((l) => l || " ");
  const out = [];
  for (const raw of ls) {
    const l = raw.replace(/\*\*|__|`/g, "").replace(/^ {0,3}#{1,6}\s+/, "");
    const item = /^\s*([-*+]|\d{1,9}[.)])\s+/.exec(l);
    if (item || !out.length) out.push(item ? l.replace(/^(\s*)[-*+]\s+/, "$1· ") : l.trim());
    else out[out.length - 1] += ` ${l.trim()}`;
  }
  return out;
}

function part(p, j) {
  const md = () => TextView.markdown(`bridge-spec-md-${j}`, p.md);
  if (p.kind === "added") {
    return h_flex().id(`bridge-spec-${j}`).w_full().flex_none().items_stretch()
      .child(div().w(4).flex_none().bg(ROLE.working))
      .child(div().flex_1().min_w(0).pl(12).child(md()));
  }
  if (p.kind === "removed") {
    return v_flex().id(`bridge-spec-${j}`).w_full().flex_none().pl(16).text_size(16).text_color(ROLE.neutral).line_through()
      .children(struck(p.md).map((l) => div().w_full().line_through().child(l)));
  }
  return div().id(`bridge-spec-${j}`).w_full().flex_none().child(md());
}

const pill = (a, r) => div().flex_none().px(6).rounded(4).bg(ROLE.bg).text_size(TEXT_MIN).font_weight(700)
  .text_color(ROLE.working).child(`+${a} −${r}`);

// a file of SPEC's list on main: like a heading row, in bold, open (black) while it is the one shown
function fileRow(f, k, on, onClick) {
  return h_flex().id(`bridge-file-${k}`).w_full().h(TOUCH).flex_none().gap(6).items_center().px(12)
    .bg(on ? ROLE.bg : ROLE.frame).text_color(on ? ROLE.frame : ROLE.ink).cursor_pointer()
    .on_click(() => onClick())
    .child(div().flex_1().min_w(0).truncate().text_size(TEXT_BODY).font_weight(700).child(up(`${on ? "▾ " : "▸ "}${f.label}`)));
}

function headRow(h, i, on, onClick, indent = false) {
  return h_flex().id(`bridge-head-${i}`).w_full().h(TOUCH).flex_none().gap(6).items_center().px(12).when(indent, (el) => el.pl(28))
    .bg(on ? ROLE.bg : ROLE.frame).text_color(on ? ROLE.frame : ROLE.ink).cursor_pointer()
    .on_click((_e, cx) => onClick(cx))
    .child(div().flex_1().min_w(0).truncate().text_size(TEXT_BODY).font_weight(700).child(up(`${on ? "▸ " : ""}${h.title}`)))
    .when(!!(h.a || h.r), (el) => el.child(pill(h.a, h.r)));
}

const headLine = (i) => div().id(`bridge-headline-${i}`).w_full().h(2).flex_none().bg(ROLE.frame)
  .child(div().size_full().bg(LINE_IN));

function specView(plan, w, onTitle, scroll) {
  const file = plan.spec;
  const name = plan.title || plan.topic;
  if (!file) return notice(`NO SPEC FOR PLAN ${name}`, ROLE.frame);
  const d = shared(DOC_KEY);
  const mine = samePlace(d, w) && d.file === file ? d : null;
  const doc = mine?.data ?? null;
  const { parts, heads, a, r } = doc ? specParts(doc) : { parts: [], heads: [], a: 0, r: 0 };
  // before /doc answers: the branch plan's headings from /space (a picked plan or file brings its own, or none)
  const early = plan.headings ?? shared(SPACE_KEY)?.data?.spec_headings ?? [];
  const listed = doc ? heads : early.map((t) => ({ title: t, child: null, a: 0, r: 0 }));
  const at = shared(AT_KEY);
  const cur = at && at.file === file && at.i < listed.length ? at.i : 0;
  const last = Math.max(0, parts.length - 1);

  // a heading click marks it and scrolls its section to the top: scroll_to_item only moves as far as needed,
  // so it first goes to the end, and a moment later back up to the heading
  const go = (i, cx) => {
    setShared(AT_KEY, { file, i });
    const child = listed[i]?.child;
    if (!scroll || child == null) return;
    if (child === 0 || !cx?.timer?.after) { scroll.scroll_to_item(child); return; }
    scroll.scroll_to_item(last);
    cx.timer.after(80, (cx) => { scroll.scroll_to_item(child); cx.notify(); });
  };

  const rows = [];
  if (plan.files) {
    // a list of files (SPEC on main): one row per file, the shown one open with its headings under it
    plan.files.forEach((f, k) => {
      if (k > 0) rows.push(headLine(`f${k}`));
      const on = f.file === file;
      rows.push(fileRow(f, k, on, () => setShared(MAINSPEC_KEY, { machine: w.machine, cwd: w.cwd, file: f.file })));
      if (on) listed.forEach((h, i) => {
        rows.push(headLine(`f${k}-${i}`));
        rows.push(headRow(h, i, i === cur, (cx) => go(i, cx), true));
      });
    });
  } else {
    listed.forEach((h, i) => {
      if (i > 0) rows.push(headLine(i));
      rows.push(headRow(h, i, i === cur, (cx) => go(i, cx)));
    });
  }

  const dirty = a + r > 0;
  const arm = h_flex().id("bridge-title").w_full().h(ARM).flex_none().bg(ROLE.frame).rounded_tl(OUTER)
    .items_center().pl(OUTER).pr(16).gap(12).text_color(ROLE.ink)
    .child(div().flex_1().min_w(0).truncate().text_size(TITLE).font_weight(700).child(up(`${plan.kind ?? "SPEC"} · ${name}`)))
    .child(v_flex().flex_none().items_end()
      .child(div().text_size(TEXT_MIN).child(file))
      .child(h_flex().gap(4).items_center().text_size(TEXT_MIN).font_weight(700)
        .child(dirty ? "WORKTREE DIRTY ·" : "NO CHANGES VS HEAD")
        .when(dirty, (el) => el.child(pill(a, r)).child("VS HEAD"))));

  const reader = v_flex().id("bridge-spec").flex_1().min_w(0).h_full().overflow_y_scrollbar()
    .when(!!scroll, (el) => el.track_scroll(scroll)).pt(10).pr(16).pb(16).gap(10)
    .when(!doc, (el) => el.child(div().text_size(TEXT_BODY).font_weight(700).text_color(mine?.error ? ROLE.alert : ROLE.neutral)
      .child(mine?.error ? up(`SPEC UNREADABLE: ${mine.error}`) : "READING SPEC …")))
    .when(!!doc && !parts.length, (el) => el.child(div().text_size(TEXT_BODY).font_weight(700).text_color(ROLE.neutral).child("SPEC IS EMPTY")))
    .children(parts.map(part));

  return v_flex().id("bridge").size_full().bg(ROLE.bg)
    .when(!!(doc && mine.error), (el) => el.child(div().w_full().pb(4).text_size(TEXT_MIN).font_weight(700).text_color(ROLE.alert)
      .truncate().child(up(`SPEC UNREADABLE: ${mine.error}`))))
    .child(onTitle ? press(arm, onTitle) : arm)
    .child(h_flex().w_full().flex_1().min_h(0).items_stretch()
      .child(v_flex().id("bridge-heads").w(SPEC_W).flex_none().h_full().overflow_y_scrollbar()
        .when(!listed.length && !plan.files, (el) => el.child(div().w_full().h(TOUCH).flex_none().bg(ROLE.frame).flex().items_center().px(12)
          .text_size(TEXT_BODY).font_weight(700).text_color(ROLE.ink).child("NO HEADINGS")))
        .children(rows)
        .child(div().id("bridge-foot").w_full().flex_1().min_h(INNER).bg(ROLE.frame).rounded_bl(INNER)))
      .child(div().w(NECK).flex_none().h_full()
        .child(div().w(NECK).h(NECK).bg(ROLE.frame).child(div().size_full().bg(ROLE.bg).rounded_tl(INNER))))
      .child(div().w(GAP).flex_none())
      .child(reader));
}

export function bridge(now, scroll) {
  const w = where();
  if (!w) return notice("NO FOLDER FOR THIS SPACE", ROLE.neutral);
  const mode = shared(BRIDGE_KEY);
  if (mode === "overview") return overviewView(w, now);
  const pick = pickFor(w);
  if (pick) return pickView(pick, w, mode, scroll);
  const st = shared(SPACE_KEY);
  if (!samePlace(st, w)) return notice("SCANNING …", ROLE.neutral);
  const data = st.data;
  if (!data) {
    const text = repoNotice(st.error);
    return notice(text, text === "NO REPO FOR THIS SPACE" ? ROLE.neutral : ROLE.alert);
  }
  const plan = data.plan;
  if (!plan) {
    // a checkout without a plan (main): PLAN reads the vision, SPEC lists every spec of the repo
    if (!pickFor(w) && (mode === "plan" || mode === "spec")) {
      const md = mainDoc(w, mode);
      if (md) return specView({ spec: md.file, title: md.title, headings: [], files: md.list ?? null,
        kind: mode === "plan" ? "VISION" : "SPEC" }, w, null, scroll);
      const ov = overviewHere(w);
      if (!ov) return notice("SCANNING …", ROLE.neutral);
      if (ov.data && mode === "plan") return notice("NO VISION FOUND (README OR CONTEXT FILE)", ROLE.frame);
      if (ov.data && mode === "spec") return notice("NO SPECS IN THIS REPO", ROLE.frame);
    }
    return notice(`NO PLAN FOR BRANCH ${data.branch || w.branch || "?"}`, ROLE.frame, st.error ? `REPO UNREADABLE: ${st.error}` : null);
  }

  const coord = plan.coordinator || data.coordinator || null;
  const onTitle = coord ? () => setBottom({ kind: "terminal", address: coord, label: `COORDINATOR · ${up(plan.title || plan.topic)}` }) : null;
  if (mode === "spec") return specView(plan, w, onTitle, scroll);
  return planView(plan, { onTitle, error: st.error, warnings: data.warnings });
}

// PLAN or SPEC for what OVERVIEW picked: a change without a worktree, read from the main checkout (/repo's view),
// read-only: no cards, no coordinator click; or the vision file in SPEC.
function pickView(pick, w, mode, scroll) {
  if (!pick.change) {
    if (mode === "spec" && pick.file) return specView({ spec: pick.file, title: pick.title || pick.file, headings: [] }, w, null, scroll);
    return notice("NOTHING PICKED", ROLE.neutral);
  }
  const r = shared(REPO_KEY);
  if (!samePlace(r, w)) return notice("SCANNING …", ROLE.neutral);
  const vp = viewPlan(w, pick.change);
  if (!vp) return notice(r.error ? repoNotice(r.error) : `NO CHANGE ${pick.change}`, r.error ? ROLE.alert : ROLE.frame);
  const plan = { ...vp, title: vp.title || vp.topic };
  if (mode === "spec") return specView({ ...plan, headings: [] }, w, null, scroll);
  return planView(plan, { error: r.error, warnings: [], readOnly: true });
}

function planView(plan, { onTitle = null, error = null, warnings = [], readOnly = false } = {}) {
  const tasks = (plan.tasks ?? []).map((t) => ({ ...t, topic: plan.topic }));
  const live = new Map(agents().map((a) => [a.address, a]));
  const chosen = shared(OPEN_KEY);
  const openN = chosen && chosen.topic === plan.topic ? chosen.n : current(tasks);

  const warn = (warnings ?? []).filter(Boolean).map(String);
  const rows = [];
  tasks.forEach((t, i) => {
    if (i > 0) rows.push(line(i));
    rows.push(row(t, openN, live, readOnly));
  });
  const title = `${plan.title || plan.topic}${readOnly ? " · READ-ONLY" : ""}`;
  return v_flex().id("bridge").size_full().bg(ROLE.bg)
    .when(!!error, (el) => el.child(div().w_full().pb(4).text_size(TEXT_MIN).font_weight(700).text_color(ROLE.alert).truncate()
      .child(up(`REPO UNREADABLE: ${error}`))))
    .when(warn.length > 0, (el) => el.child(div().w_full().pb(4).text_size(TEXT_MIN).font_weight(700).text_color(ROLE.alert).truncate()
      .child(up(warn.join(" · ")))))
    .child(header(title, onTitle))
    .child(neck())
    .child(v_flex().id("bridge-rows").w_full().flex_1().min_h(0).overflow_y_scrollbar()
      .when(!tasks.length, (el) => el.child(elbowCell(div().flex_1().h(ROW).bg(ROLE.frame).flex().items_center().px(12)
        .text_size(TEXT_BODY).font_weight(700).text_color(ROLE.ink).child("NO TASKS IN PLAN"))))
      .children(rows)
      .child(foot()));
}

// --- OVERVIEW mode ---------------------------------------------------------------------------------
// PLAN one level up (mockup overview-layout-v2): the arm carries the repo and the briefing; the elbow's rows are
// Vision, the changes, Register and Done, then the capabilities; the cards are the coordinators.

// an oval: filled with `color`, or only outlined (a planned change)
const ovOval = (color, blink = false) => div().w(24).h(14).flex_none().rounded(7).border(color ? 1 : 2).border_color(ROLE.ink)
  .when(!!color, (el) => el.bg(color)).when(blink, (el) => el.loop("opacity", { from: 0.4, to: 1, period: 700 }));

// a small button on the elbow (round ends: "click me"): black with orange text on the orange elbow, orange with
// black text on an open (black) row so it stays visible. The pill sits in a TOUCH × TOUCH (at least) hit box, whose
// press does not reach the row under it.
const ovButton = (id, word, fn, onBlack = false) => h_flex().id(id).h(TOUCH).min_w(TOUCH).flex_none().items_center()
  .justify_center().cursor_pointer().hover((s) => s.opacity(0.85))
  .on_mouse_down("left", (_e, cx) => cx.stop_propagation())
  .on_click((_e, cx) => { cx?.stop_propagation?.(); fn(); })
  .child(h_flex().h(32).flex_none().px(14).rounded(16).items_center().bg(onBlack ? ROLE.frame : ROLE.bg)
    .text_size(TEXT_MIN).font_weight(700).text_color(onBlack ? ROLE.ink : ROLE.frame).child(word));

// a card on a row: status strip, then its lines on black; clickable when it opens a terminal
function ovCard(id, color, lines, { blink = false, bar = null, onClick = null } = {}) {
  const el = h_flex().id(id).size_full().gap(8).items_stretch()
    .child(div().w(8).flex_none().bg(color).when(blink, (e) => e.loop("opacity", { from: 0.4, to: 1, period: 700 })))
    .child(v_flex().flex_1().min_w(0).justify_center().py(3)
      .children(lines.map(([text, c, big], k) => div().id(`${id}-l${k}`).text_size(big ? TEXT_BODY : TEXT_MIN).font_weight(700)
        .text_color(c).truncate().child(text)))
      .when(!!bar, (e) => e.child(bar)));
  return onClick ? press(el, onClick) : el;
}

const tickBar = (done, total, color = ROLE.working) => stepBar({ steps_done: done, steps_total: total }, color);

// the coordinator's status (Striker's /status, polled by the sidebar): by its terminal, else by change in this repo
function coordStatus(change, path) {
  let byName = null;
  for (const r of shared(FLEET_KEY)?.repos ?? []) {
    for (const c of r.coordinators ?? []) {
      if (change.agent && c.address === change.agent) return c.status ?? null;
      if (c.change === change.name && (r.path === path || r.space_path === path)) byName = c.status ?? null;
    }
  }
  return byName;
}

// a change's card: NEEDS YOU (the coordinator's question), WORKING (coordinator, its task, the ticks; or a task
// agent or the lead on it, no click), INTERRUPTED (neutral, the last ticked task, no click), READY TO ARCHIVE (gold,
// every task ticked, nobody on it)
function changeCard(c, items, vp, path, i) {
  const cc = coordCard(c, items);
  if (!cc) return null;
  const name = up(c.name);
  const id = `bridge-ov-card-${i}`;
  const coord = c.agent ? (alert) => setBottom({ kind: "terminal", address: c.agent, label: `COORDINATOR · ${name}`, alert }) : null;
  const total = Number(c.total) || 0, done = Number(c.done) || 0;
  if (cc.col === "needs_you") {
    const ask = cc.asks[0]?.title ?? "NEEDS YOU";
    return { col: cc.col, el: ovCard(id, ROLE.alert, [[name, ROLE.text, true], ["COORD ASKS:", ROLE.alert], [ask, ROLE.text]],
      { blink: true, onClick: coord ? () => coord(true) : null }) };
  }
  if (cc.kind === "interrupted") {
    return { col: cc.col, el: ovCard(id, ROLE.neutral, [[name, ROLE.text, true], [interruptedText(c, vp) ?? "INTERRUPTED", ROLE.neutral],
      [`${done}/${total} TICKED · NO COORDINATOR`, ROLE.neutral]], { bar: tickBar(done, total, ROLE.neutral) }) };
  }
  if (cc.kind === "live") {
    const at = vp ? nowTask(vp.tasks) : null;
    const task = at ? `TASK ${at.id ?? at.n}` : "";
    return { col: cc.col, el: ovCard(id, ROLE.working, [[name, ROLE.text, true], [[task, "WORKING"].filter(Boolean).join(" · "), ROLE.working],
      [`${done}/${total} TICKED · NO COORDINATOR`, ROLE.neutral]], { bar: tickBar(done, total) }) };
  }
  if (cc.col === "working") {
    const st = coordStatus(c, path);
    const task = st?.task ? up(st.task) : vp ? `TASK ${nowTask(vp.tasks)?.id ?? "?"}` : "";
    return { col: cc.col, el: ovCard(id, ROLE.working, [[name, ROLE.text, true], [`COORD · ${task} · WORKING`, ROLE.working],
      ["CLICK: COORDINATOR BELOW", ROLE.neutral]], { bar: tickBar(done, total), onClick: coord ? () => coord(false) : null }) };
  }
  return { col: cc.col, el: ovCard(id, ROLE.done, [[name, ROLE.text, true], [READY_TEXT, ROLE.done],
    [`ALL ${total} TASKS TICKED`, ROLE.done]]) };
}

// one row: the elbow part (head + open body) and its card in one of the three columns; `extra(open)` draws a
// button on the head (it knows whether the head is black)
function ovRow(i, { open, onToggle, oval, label, extra = null, body = null, card = null }) {
  const head = h_flex().id(`bridge-ov-head-${i}`).w_full().h(ROW_OV).flex_none().gap(10).items_center().px(12)
    .bg(open ? ROLE.bg : ROLE.frame).text_color(open ? ROLE.active : ROLE.ink).cursor_pointer()
    .on_click(() => onToggle())
    .child(oval)
    .child(div().flex_1().min_w(0).truncate().text_size(TEXT_BODY).font_weight(700).child(up(label)))
    .when(!!extra, (el) => el.child(extra(open)))
    .child(div().flex_none().text_size(TEXT_BODY).font_weight(700).child(open ? "▾" : "▸"));
  const frame = v_flex().flex_1().min_w(0).bg(ROLE.frame).child(head).when(open && !!body, (el) => el.child(body));
  return h_flex().id(`bridge-ov-row-${i}`).w_full().flex_none().items_stretch()
    .child(elbowCell(frame))
    .child(div().w(GAP).flex_none())
    .child(h_flex().flex_1().min_w(0).gap(GAP).items_stretch().children(COLUMNS.map(([st]) =>
      v_flex().flex_1().min_w(0).child(card && card.col === st ? card.el : div()))));
}

// an open row's lines: plain black text on the elbow
const ovBody = (i, kids) => v_flex().id(`bridge-ov-body-${i}`).w_full().px(12).pt(4).pb(8).gap(2).text_color(ROLE.ink).children(kids);
const ovLine = (id, text, { now = false, dim = false, size = TEXT_BODY } = {}) => div().id(id).w_full().px(6).py(1).text_size(size)
  .when(now, (el) => el.bg(ROLE.bg).text_color(ROLE.active).font_weight(700).rounded(4))
  .when(dim && !now, (el) => el.text_color(`${ROLE.ink}99`))
  .child(text);

// an open change: its table of contents (proposal/design headings), then its tasks with ticks and ◂ NOW
function changeBody(i, c, vp, w) {
  const toc = shared(TOC_KEY);
  const mine = samePlace(toc, w) && toc.change === c.name ? toc : null;
  const kids = [];
  const t = tocLine(mine, vp); // /doc's labelled headings, else /repo's flat toc while /doc loads or fails
  kids.push(ovLine(`bridge-ov-toc-${i}`, t || (mine ? "NO HEADINGS" : "READING CONTENTS …"), { size: TEXT_MIN, dim: true }));
  const tasks = vp?.tasks ?? [];
  const at = nowTask(tasks);
  tasks.forEach((x, k) => {
    const mark = x.state === "done" ? "☑" : "☐";
    kids.push(ovLine(`bridge-ov-task-${i}-${k}`, `${mark} ${x.id} ${x.title}${x === at ? "  ◂ NOW" : ""}`,
      { now: x === at, dim: x.state === "done" }));
  });
  if (!vp) kids.push(ovLine(`bridge-ov-notasks-${i}`, "TASKS: SCANNING REPO …", { size: TEXT_MIN }));
  else if (!tasks.length) kids.push(ovLine(`bridge-ov-notasks-${i}`, "NO TASKS", { size: TEXT_MIN }));
  return ovBody(i, kids);
}

// a click on a change: its worktree's own space (selected, PLAN there), else PLAN read-only from the main checkout
function openChange(c, legacy, w, repo) {
  const sp = legacy ? null : changeSpace(c, spaces(), repo, w.machine);
  if (sp) {
    run("space.select", { id: sp.id });
    showBridge("plan");
    return;
  }
  setShared(PICK_KEY, { machine: w.machine, cwd: w.cwd, change: c.name, title: c.title || c.name });
  showBridge("plan");
}

function overviewView(w, now) {
  const o = overviewHere(w);
  if (!o) return notice("SCANNING …", ROLE.neutral);
  const ov = o.data;
  if (!ov) {
    const text = repoNotice(o.error);
    return notice(text, text === "NO REPO FOR THIS SPACE" ? ROLE.neutral : ROLE.alert);
  }
  const repo = repoName(shared(FLEET_KEY)?.repos ?? [], w.machine, o.path ?? w.cwd);
  const rows = overviewRows(ov);
  const open = openRow(w, rows);
  const toggle = (key) => () => setShared(OV_OPEN_KEY, { machine: w.machine, cwd: w.cwd, key: open === key ? null : key });
  const items = ov.open_items ?? [];

  const els = [];
  rows.forEach((r, i) => {
    if (i > 0) els.push(line(`ov-${i}`));
    const isOpen = open === r.key;
    if (r.kind === "vision") {
      const ctx = r.context;
      const body = ovBody(i, [
        ovLine("bridge-ov-lead", ctx?.lead || "NO VISION FOUND (README OR CONTEXT FILE)"),
        ...(ctx?.file ? [h_flex().w_full().px(6).pt(4).child(ovButton("bridge-ov-spec", `${up(ctx.file)} IN SPEC ▸`, () => {
          setShared(PICK_KEY, { machine: w.machine, cwd: w.cwd, file: ctx.file, title: ctx.title || ctx.file });
          showBridge("spec");
        }))] : []),
      ]);
      els.push(ovRow(i, { open: isOpen, onToggle: toggle(r.key), oval: ovOval(ROLE.ink), label: `VISION · ${ctx?.title || "—"}`, body }));
    } else if (r.kind === "change") {
      const c = r.change;
      const vp = viewPlan(w, c.name);
      const after = afterText(c, vp);
      const ready = changeStatus(c) === "ready" ? READY_TEXT : null;
      const label = [c.name, `${Number(c.done) || 0}/${Number(c.total) || 0}`, after, ready].filter(Boolean).join(" · ");
      els.push(ovRow(i, { open: isOpen, onToggle: toggle(r.key), oval: ovOval(changeColor(c), c.state === "needs_you"), label,
        extra: (onBlack) => ovButton(`bridge-ov-plan-${i}`, "PLAN ▸", () => openChange(c, r.legacy, w, repo), onBlack),
        body: changeBody(i, c, vp, w), card: changeCard(c, items, vp, o.path ?? w.cwd, i) }));
    } else if (r.kind === "register") {
      const n = r.items.length;
      const body = ovBody(i, n ? r.items.map((it, k) => ovLine(`bridge-ov-item-${k}`, itemLabel(it))) : [ovLine("bridge-ov-noitems", "NO OPEN ITEMS")]);
      const card = n ? { col: "needs_you", el: ovCard(`bridge-ov-card-${i}`, ROLE.alert, [[registerSummary(r.items), ROLE.text, true]], { blink: true }) } : null;
      els.push(ovRow(i, { open: isOpen, onToggle: toggle(r.key), oval: ovOval(n ? ROLE.alert : ROLE.neutral, n > 0),
        label: `REGISTER · ${n ? `${n} OPEN ITEM${n > 1 ? "S" : ""}` : "NO OPEN ITEMS"}`, body, card }));
    } else if (r.kind === "done") {
      const list = newestFirst(r.archived);
      const body = ovBody(i, list.length ? list.map((a, k) => ovLine(`bridge-ov-arch-${k}`, `☑ ${a.name}${a.date ? ` · ${a.date}` : ""}`))
        : [ovLine("bridge-ov-noarch", "NOTHING ARCHIVED YET")]);
      const sum = doneSummary(list);
      const card = sum ? { col: "done", el: ovCard(`bridge-ov-card-${i}`, ROLE.done, [[sum, ROLE.text, true]]) } : null;
      els.push(ovRow(i, { open: isOpen, onToggle: toggle(r.key), oval: ovOval(list.length ? ROLE.done : ROLE.neutral),
        label: `DONE · ${list.length ? list.map((a) => a.name).join(" · ") : "NONE"}`, body, card }));
    }
  });
  const caps = (ov.inventory?.capabilities ?? []).map((c) => String(c?.path ?? "").replace(/\/+$/, "").split("/").pop()).filter(Boolean);
  els.push(elbowCell(div().id("bridge-ov-caps").flex_1().min_w(0).bg(ROLE.frame).px(12).pt(6).pb(2).text_size(TEXT_MIN).font_weight(700)
    .text_color(ROLE.ink).truncate().child(`CAPABILITIES: ${caps.length ? caps.map(up).join(" · ") : "NONE YET"}`)));

  const warn = [...(ov.warnings ?? []).filter(Boolean).map(String)];
  const arm = v_flex().id("bridge-title").w(LEFT).h_full().flex_none().bg(ROLE.frame).rounded_tl(OUTER)
    .justify_center().pl(OUTER).pr(12).text_color(ROLE.ink)
    .child(div().min_w(0).truncate().text_size(TITLE).font_weight(700).child(up(repo)))
    .child(div().min_w(0).truncate().text_size(TEXT_BODY).child(briefingLine(ov.briefing, now ?? Date.now())))
    .child(div().id("bridge-ov-commits").min_w(0).truncate().text_size(TEXT_MIN).child(commitsLine(ov.briefing)));
  return v_flex().id("bridge").size_full().bg(ROLE.bg)
    .when(!!o.error, (el) => el.child(div().w_full().pb(4).text_size(TEXT_MIN).font_weight(700).text_color(ROLE.alert).truncate()
      .child(up(`OVERVIEW UNREADABLE: ${o.error}`))))
    .when(warn.length > 0, (el) => el.child(div().w_full().pb(4).text_size(TEXT_MIN).font_weight(700).text_color(ROLE.alert).truncate()
      .child(up(warn.join(" · ")))))
    .child(h_flex().w_full().h(ARM_OV).flex_none().gap(GAP)
      .child(arm)
      .child(columnHeads()))
    .child(neck())
    .child(v_flex().id("bridge-rows").w_full().flex_1().min_h(0).overflow_y_scrollbar()
      .children(els)
      .child(foot()));
}

// The bottom field's rule (its own layout block, so it stays while the block under it shows a terminal): its
// label, and a round ◂ REPO button while a terminal is shown.
export function bridgeRule(_now) {
  const b = shared(BOTTOM_KEY);
  const term = b?.kind === "terminal";
  const color = ruleColor(b);
  return h_flex().id("bridge-rule").size_full().bg(ROLE.bg).gap(6).items_center()
    .child(div().w(18).h(2).flex_none().bg(color))
    .child(div().flex_none().text_size(TEXT_MIN).font_weight(700).text_color(color).child(term ? up(b.label) : "REPO"))
    .child(div().flex_1().h(2).bg(color))
    .when(term, (el) => el.child(press(h_flex().id("bridge-back").h(TOUCH).min_w(TOUCH).flex_none().px(16).rounded(TOUCH / 2)
      .bg(ROLE.frame).items_center().justify_center().text_size(TEXT_MIN).font_weight(700).text_color(ROLE.ink).child("◂ REPO"),
    () => setBottom({ kind: "repo" }))));
}

// --- the REPO tree --------------------------------------------------------------------------------

// Herdr's zoom scales element px but not path geometry nor on_size_change sizes (as blueprint found): the tree
// is laid out in layout px and its paths scaled by the zoom.
const zoomOf = () => { const z = Number(settings()?.zoom); return z > 0 ? z : 1; };
const alpha = (color, a) => (a >= 1 ? color : `${color}${Math.round(a * 255).toString(16).padStart(2, "0")}`);

function circle(b, x, y, r) {
  return b.move_to(x + r, y).arc_to(r, r, 0, false, true, x - r, y).arc_to(r, r, 0, false, true, x + r, y).close();
}

// The tree for a /repo graph in a box `width` × `height` (layout px).
export function drawTree(graph, width, height = 132, opts = {}) {
  const k = zoomOf();
  const t = treeLayout(graph, { width, height, plans: opts.plans ?? [], topic: opts.topic ?? null });
  const layer = (path, color) => div().absolute().left(0).top(0).w(width).h(height).child(window.paint_path(path, color));
  const kids = [];
  // NOW: a dotted rule and its tag
  const nowRule = PathBuilder.stroke(1 * k).dash_array(DOTS.map((v) => v * k)).move_to(t.nowX * k, NOW_H * k).line_to(t.nowX * k, height * k);
  kids.push(layer(nowRule.build(), ROLE.frame));
  for (const s of t.strokes) {
    let b = PathBuilder.stroke(s.width * k);
    if (s.dash) b = b.dash_array(s.dash.map((v) => v * k));
    for (const g of s.segs) {
      const [[x1, y1], [x2, y2]] = g.pts;
      b = b.move_to(x1 * k, y1 * k);
      if (g.kind === "curve") {
        const mx = (x1 + x2) / 2;
        b = b.cubic_bezier_to(x2 * k, y2 * k, mx * k, y1 * k, mx * k, y2 * k);
      } else b = b.line_to(x2 * k, y2 * k);
    }
    kids.push(layer(b.build(), alpha(s.color, s.alpha)));
  }
  for (const m of t.marks) {
    if (m.fill) { kids.push(layer(circle(PathBuilder.fill(), m.x * k, m.y * k, m.r * k).build(), alpha(m.fill, m.alpha))); continue; }
    kids.push(layer(circle(PathBuilder.fill(), m.x * k, m.y * k, m.r * k).build(), ROLE.bg));
    let b = PathBuilder.stroke((m.dash ? 1.5 : 2.5) * k);
    if (m.dash) b = b.dash_array([2 * k, 2 * k]);
    kids.push(layer(circle(b, m.x * k, m.y * k, m.r * k).build(), alpha(m.ring, m.alpha)));
  }
  // the lane labels: MAIN left of the trunk, a change's label where its branch starts (right-aligned)
  for (const l of t.labels) {
    kids.push(div().absolute().left(l.x).top(l.y).w(l.w).h(LABEL_H).overflow_hidden().flex().items_center()
      .when(l.align === "right", (el) => el.justify_end())
      .text_size(LABEL_TEXT).font_weight(700).text_color(alpha(l.color, l.alpha))
      .child(div().min_w(0).truncate().whitespace_nowrap().child(l.text)));
  }
  if (t.hidden) {
    kids.push(div().absolute().right(4).bottom(2).px(8).rounded(9).bg(ROLE.neutral).text_size(TEXT_MIN).font_weight(700)
      .text_color(ROLE.ink).child(`+${t.hidden} LANE${t.hidden > 1 ? "S" : ""}`));
  }
  kids.push(div().absolute().left(t.nowX - 22).top(0).w(44).h(NOW_H).bg(ROLE.frame).flex().items_center().justify_center()
    .text_size(TEXT_MIN).font_weight(700).text_color(ROLE.ink).child("NOW"));
  return div().id("bridge-tree").relative().w(width).h(height).overflow_hidden().children(kids);
}

// The bottom block while it shows the tree. `box` is the copy's own { w, h } (real px from on_size_change).
export function bridgeBottom(_now, box = {}) {
  const k = zoomOf();
  const w = (box.w ?? 0) / k, h = (box.h ?? 0) / k;
  const where_ = where();
  const r = shared(REPO_KEY);
  const mine = where_ && samePlace(r, where_) ? r : null;
  const graph = mine?.data?.graph ?? mine?.data?.view?.graph ?? null;
  const msg = (text, color) => div().size_full().flex().items_center().justify_center().text_size(TEXT_BODY).font_weight(700)
    .text_color(color).child(text);
  let body;
  if (!where_) body = msg("NO FOLDER FOR THIS SPACE", ROLE.neutral);
  else if (!mine) body = msg("SCANNING REPO …", ROLE.neutral);
  else if (!graph && mine.error && repoNotice(mine.error) === "NO REPO FOR THIS SPACE") body = msg("NO REPO FOR THIS SPACE", ROLE.neutral);
  else if (!graph) body = msg(up(`REPO UNREADABLE: ${mine.error ?? "NO TREE IN ANSWER"}`), ROLE.alert);
  else if (w > 0 && h > 0) {
    const st = shared(SPACE_KEY);
    const topic = samePlace(st, where_) ? st.data?.plan?.topic ?? st.data?.topic ?? null : null;
    body = drawTree(graph, w, h, { plans: mine.data?.view?.plans ?? [], topic });
  } else body = div();
  return div().id("bridge-bottom").size_full().bg(ROLE.bg).relative()
    .on_size_change((size, cx) => {
      if (size.width !== box.w || size.height !== box.h) { box.w = size.width; box.h = size.height; cx.notify(); }
    })
    .child(body)
    .when(!!(graph && mine?.error), (el) => el.child(div().absolute().right(0).bottom(0).px(4).bg(ROLE.bg).text_size(TEXT_MIN)
      .font_weight(700).text_color(ROLE.alert).child(up(`REPO UNREADABLE: ${mine.error}`))));
}
