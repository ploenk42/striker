// The OVERVIEW's pure parts (no gpui imports, so node can check them: bridge.check.mjs). bridge.js draws.
// In: Striker's /overview answer ({briefing, context, inventory, open_items, warnings}), /repo's `view.plans` for a
// change's tasks and table of contents, herdr's spaces and the fleet from /status. Open items may lack `id` and
// carry a `kind`.

import { ROLE, WORD } from "./palette.js";

const up = (s) => String(s ?? "").toUpperCase();
const arr = (x) => (Array.isArray(x) ? x : []);
const machineOf = (m) => (m == null || m === "" || m === "local" ? null : String(m));
const trim = (p) => String(p ?? "").replace(/\\/g, "/").replace(/\/+$/, "");

// legacy plan states (docs/superpowers) in OpenSpec's words
const LEGACY_STATE = { active: "in progress", merged: "done" };

// The elbow's rows, top down: Vision, one per active change (legacy plans of a repo without openspec/ likewise),
// Register (the lead's needs_you and the REGISTER; a coordinator's items sit on its change's row), Done.
export function overviewRows(ov) {
  const inv = ov?.inventory ?? {};
  const rows = [{ kind: "vision", key: "vision", context: ov?.context ?? null }];
  for (const c of arr(inv.changes)) if (c?.name) rows.push({ kind: "change", key: `change:${c.name}`, change: c, legacy: false });
  const doneLegacy = [];
  for (const l of arr(inv.legacy)) {
    if (!l?.name) continue;
    const state = LEGACY_STATE[l.state] ?? l.state ?? "planned";
    if (state === "done") { doneLegacy.push({ name: l.name, date: "" }); continue; }
    rows.push({ kind: "change", key: `change:${l.name}`, legacy: true, change: { name: l.name, title: l.title ?? "", state,
      done: Number(l.done) || 0, total: Number(l.total) || 0, worktree: null, agent: null, interrupted_at: "", depends_on: [], branch: "" } });
  }
  // a coordinator's items sit on its change's row; with no such row (no id, unknown change) they stay here
  const names = new Set(rows.filter((r) => r.kind === "change").map((r) => r.change.name));
  const items = arr(ov?.open_items).filter((i) => i && (i.source !== "coordinator" || !i.id || !names.has(i.id)));
  rows.push({ kind: "register", key: "register", items });
  rows.push({ kind: "done", key: "done", archived: [...arr(inv.archived), ...doneLegacy] });
  return rows;
}

// Whether someone works on a change now: its coordinator (`agent`), else Striker's `live` (a task agent of its topic,
// or the lead working on it). An older striker serve sends no `live`: then a task working or waiting on the person,
// or a change in progress with ticked and open tasks that Striker did not call interrupted.
export function changeLive(change) {
  if (!change) return false;
  if (change.agent || change.coordinator) return true; // /overview's change, /repo's plan
  if (typeof change.live === "boolean") return change.live;
  const tasks = arr(change.tasks);
  if (tasks.some((t) => t?.state === "working" || t?.state === "needs_you")) return true;
  const total = Number(change.total) || 0, done = Number(change.done) || 0;
  return change.state === "in progress" && !change.interrupted_at && !tasks.some((t) => t?.state === "interrupted")
    && done > 0 && done < total;
}

// What is happening with a change, the one rule for its oval, its card and its tree lane:
// needs_you (it asks the person) > done (archived) > blocked > working (someone is on it) > ready (every task
// ticked, not archived: READY TO ARCHIVE) > interrupted (ticked and open tasks, nobody on it) > planned (no ticks).
// `done`/`total` are the ticked and all tasks (a /repo plan without them counts its `tasks`).
export function changeStatus(change) {
  if (!change) return "planned";
  if (change.state === "needs_you") return "needs_you";
  if (change.state === "done" || change.state === "merged") return "done";
  if (change.state === "blocked") return "blocked";
  if (changeLive(change)) return "working";
  const tasks = arr(change.tasks);
  const total = change.total != null ? Number(change.total) || 0 : tasks.length;
  const done = change.done != null ? Number(change.done) || 0 : tasks.filter((t) => t?.state === "done").length;
  if (total > 0 && done >= total) return "ready";
  if (done > 0 || change.interrupted_at) return "interrupted";
  return "planned";
}

export const READY_TEXT = "READY TO ARCHIVE";

// Which column a change's card sits in, and what kind of card: NEEDS YOU (its coordinator asks, or the change is
// needs_you) > WORKING (its coordinator runs: "coord"; someone else is on it: "live") > INTERRUPTED (a neutral
// card in WORKING) > READY TO ARCHIVE (every task ticked, in DONE). null: none (planned, blocked).
export function coordCard(change, items = []) {
  if (!change) return null;
  const asks = arr(items).filter((i) => i?.source === "coordinator" && i.id === change.name);
  if (change.state === "needs_you" || asks.length) return { col: "needs_you", kind: "coord", asks };
  const st = changeStatus(change);
  if (st === "working") return { col: "working", kind: change.agent ? "coord" : "live", asks };
  if (st === "interrupted") return { col: "working", kind: "interrupted", asks };
  if (st === "ready") return { col: "done", kind: "ready", asks };
  return null;
}

// A change's oval: blue only while someone is on it, gold when ready to archive, INTERRUPTED and blocked neutral
// (alert only when it needs the person); planned is an outline (null).
const STATUS_COLOR = { needs_you: ROLE.alert, done: ROLE.done, ready: ROLE.done, working: ROLE.working,
  interrupted: ROLE.neutral, blocked: ROLE.neutral, planned: null };
export function changeColor(change) {
  const c = STATUS_COLOR[changeStatus(change)];
  return c === undefined ? ROLE.neutral : c;
}

// A PLAN task's card: every task has one on its row. WORKING and NEEDS YOU as the agent reports; INTERRUPTED (the
// first open task after ticks, nobody on it) a neutral card in WORKING; OPEN (not started) a neutral outline in
// WORKING; DONE a gold card in DONE. Only WORKING and NEEDS YOU with an agent are clickable (addr.js cardBottom).
export function planCard(t) {
  switch (t?.state) {
    case "working": return { col: "working", kind: "working", color: ROLE.working, outline: false };
    case "needs_you": return { col: "needs_you", kind: "needs_you", color: ROLE.alert, outline: false };
    case "interrupted": return { col: "working", kind: "interrupted", color: ROLE.neutral, outline: false };
    case "done": return { col: "done", kind: "done", color: ROLE.done, outline: false };
    // an open task has no card (col null): it stays in the orange list with an outline oval
    default: return { col: null, kind: "open", color: ROLE.neutral, outline: true };
  }
}

// INTERRUPTED with the last ticked task before the interrupted one (from /repo's plan), null if not interrupted.
export function interruptedText(change, plan) {
  const at = change?.interrupted_at;
  if (!at) return null;
  const tasks = arr(plan?.tasks);
  const i = tasks.findIndex((t) => t?.id === at);
  const last = (i >= 0 ? tasks.slice(0, i) : tasks).filter((t) => t?.state === "done").pop();
  return last ? `INTERRUPTED · LAST DONE ${last.id} ${up(last.title)}` : `INTERRUPTED AT TASK ${at}`;
}

// The task the change is on (◂ NOW): working, else waiting on the person, else interrupted, else the first todo.
export function nowTask(tasks) {
  const list = arr(tasks);
  for (const st of ["working", "needs_you", "interrupted", "todo"]) {
    const t = list.find((x) => x?.state === st);
    if (t) return t;
  }
  return null;
}

// "AFTER X" while a change is blocked: /repo's blocked_by (unarchived dependencies), else its depends_on.
export function afterText(change, plan) {
  if (change?.state !== "blocked") return null;
  const deps = arr(plan?.blocked_by).length ? arr(plan.blocked_by) : arr(change.depends_on);
  return deps.length ? `AFTER ${deps.map(up).join(", ")}` : null;
}

export function ageText(iso, now) {
  const t = Date.parse(iso ?? "");
  if (!Number.isFinite(t)) return "";
  const min = Math.max(0, Math.floor((now - t) / 60000));
  if (min < 1) return "just now";
  if (min < 60) return `${min} min ago`;
  const h = Math.floor(min / 60);
  return h < 48 ? `${h} h ago` : `${Math.floor(h / 24)} d ago`;
}

function progressText(p) {
  if (typeof p === "string") return p;
  if (p && typeof p === "object" && Number.isFinite(p.done) && Number.isFinite(p.total)) return `${p.done}/${p.total}`;
  return "";
}

// The arm's briefing line: phase, next step, progress, note, the lead's state and the status file's age;
// `NO LEAD STATUS` first when the lead wrote none (the commits have their own line: commitsLine).
export function briefingLine(b, now) {
  const parts = [];
  const p = progressText(b?.progress);
  if (!b?.phase && !b?.task && !p && !b?.note && !b?.updated) parts.push("NO LEAD STATUS");
  if (b?.phase) parts.push(`Phase ${b.phase}`);
  if (b?.task) parts.push(b.task);
  if (p) parts.push(p);
  if (b?.note) parts.push(b.note);
  if (b?.address) {
    const state = b.state ? String(WORD[b.state] ?? b.state).toLowerCase() : "";
    parts.push(`lead ${b.address}${state ? ` ${state}` : ""}`);
  } else parts.push("no lead");
  const age = ageText(b?.updated, now);
  if (age) parts.push(age);
  return parts.join(" · ");
}

// The briefing's last commits on the default branch (Striker serves up to 5, newest first).
export function commitsLine(b) {
  const subjects = arr(b?.commits).map((c) => c?.subject).filter(Boolean).slice(0, 5);
  return subjects.length ? `Last commits: ${subjects.join(" · ")}` : "NO COMMITS";
}

// One open item as a line: its id (register entries), else where it comes from, and its kind when given.
export function itemLabel(i) {
  const head = i?.id ? [i.id] : [up(i?.source || "item")];
  if (i?.kind && !i?.id) head.push(up(i.kind));
  return `${head.join(" · ")} · ${i?.title ?? ""}`;
}

// The Register row's NEEDS YOU card: the entries' ids, then how many items without one.
export function registerSummary(items) {
  const list = arr(items);
  const ids = list.filter((i) => i?.id).map((i) => i.id);
  const rest = list.length - ids.length;
  const head = ids.slice(0, 4).join(" · ") + (ids.length > 4 ? ` +${ids.length - 4}` : "");
  const lead = rest ? `${rest} LEAD` : "";
  return [head, lead].filter(Boolean).join(" · ");
}

// A markdown file's `##` headings (an open change row's table of contents); fenced code is skipped.
export function docHeadings(text) {
  const out = [];
  let fence = false;
  for (const l of String(text ?? "").split(/\r?\n/)) {
    if (/^ {0,3}(```|~~~)/.test(l)) { fence = !fence; continue; }
    if (!fence && /^ {0,3}##\s+/.test(l)) out.push(l.replace(/^ {0,3}##\s+/, "").replace(/\s+#+\s*$/, "").replace(/\s*\{#[^}]*\}\s*$/, "").trim());
  }
  return out.filter(Boolean);
}

// The table-of-contents line: "proposal: A · B | design: C · D" for the parts that have headings.
export function tocText(toc) {
  return arr(toc?.parts).filter((p) => arr(p?.heads).length).map((p) => `${p.label}: ${p.heads.join(" · ")}`).join("  |  ");
}

// The open change row's TOC line: /doc's proposal/design headings, else /repo's `toc` (the same headings, flat)
// while /doc loads or when it fails.
export function tocLine(toc, plan) {
  return tocText(toc) || arr(plan?.toc).filter(Boolean).join(" · ");
}

// A tick/step bar's segment colours: done ones in `color`, the next one `active` on a working bar (in `color`
// otherwise: an INTERRUPTED card stays neutral), the rest `track`.
export function barColors(done, total, color = ROLE.working) {
  const next = color === ROLE.working ? ROLE.active : color;
  return Array.from({ length: total }, (_, i) => (i < done ? color : i === done ? next : ROLE.track));
}

// The Bridge's mode when it first opens in a place: OVERVIEW on a main checkout, unless the person chose a mode
// (pseudo-tab or command); otherwise the mode it was opened with.
export function firstMode(mode, main, chosen) {
  return main && !chosen ? "overview" : mode;
}

// The archived changes, newest first (dates as YYYY-MM-DD sort as text; undated last, in their order).
export function newestFirst(list) {
  return arr(list).map((a, i) => ({ a, i })).sort((x, y) => String(y.a?.date ?? "").localeCompare(String(x.a?.date ?? "")) || x.i - y.i)
    .map((x) => x.a);
}

// The Done row's DONE card: how many, and the newest date.
export function doneSummary(list) {
  const l = newestFirst(list);
  if (!l.length) return "";
  return l[0]?.date ? `${l.length} DONE · ${l[0].date}` : `${l.length} DONE`;
}

// The row opened when the person has not picked one in this place: the first change in progress or needing them.
export function defaultOpen(rows) {
  return arr(rows).find((r) => r.kind === "change" && (r.change.state === "in progress" || r.change.state === "needs_you"))?.key ?? null;
}

// git rev-parse --git-common-dir run at `path`: the main checkout's is `<path>/.git` (printed relative, ".git"),
// a linked worktree's lies in the main checkout.
export function isMainCheckout(path, out) {
  const p = trim(path), o = String(out ?? "").trim().replace(/\\/g, "/");
  if (!p || !o) return false;
  const abs = /^([A-Za-z]:)?\//.test(o) ? o : `${p}/${o.replace(/^\.\//, "")}`;
  return trim(abs) === `${p}/.git`;
}

export const defaultMode = (main) => (main ? "overview" : "plan");

// SPEC on a main checkout: every spec of the repo as { label, file }, from /overview's inventory: the living
// specs (openspec/specs/<path>/spec.md), the active changes' design.md, then legacy specs.
export function specList(ov) {
  const inv = ov?.inventory ?? {};
  const out = [];
  for (const c of arr(inv.capabilities)) {
    if (c?.path) out.push({ label: String(c.path).split("/").join(" / "), file: `openspec/specs/${c.path}/spec.md` });
  }
  for (const c of arr(inv.changes)) {
    if (c?.name) out.push({ label: `CHANGE · ${c.name}`, file: `openspec/changes/${c.name}/design.md` });
  }
  for (const l of arr(inv.legacy)) {
    if (l?.spec) out.push({ label: l.title || l.name, file: l.spec });
  }
  return out;
}

// The space of a change's worktree: the one whose folder is the worktree, else the one `striker start-coordinator`
// names `<repo>-<change>`, on the repo's machine. null: no worktree, or no space for it.
export function changeSpace(change, list, repo, machine = null) {
  if (!change?.worktree) return null;
  const m = machineOf(machine);
  const mine = arr(list).filter((s) => machineOf(s?.machine) === m);
  const wt = trim(change.worktree);
  return mine.find((s) => trim(s.folder) === wt) ?? (repo ? mine.find((s) => s.name === `${repo}-${change.name}`) : null) ?? null;
}

// The repo's name: its fleet entry (Striker's /status) at that path, else the folder's name.
export function repoName(fleet, machine, path) {
  const p = trim(path), m = machineOf(machine);
  const r = arr(fleet).find((x) => machineOf(x?.machine) === m && (trim(x.path) === p || trim(x.space_path) === p));
  return r?.name ?? (p.split("/").pop() || p);
}
