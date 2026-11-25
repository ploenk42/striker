// Terminal addresses, compared across forms (no gpui imports, so node can check it: bridge.check.mjs).
// A terminal on this computer is "t7"; one on another machine "remote1/t7", or a bare "t7" when the machine is
// known some other way (the space's machine). "local" and "" name this computer.

import { ROLE } from "./palette.js";

// What a task card opens in the bottom field: its agent's terminal while the task is WORKING or NEEDS YOU
// (`alert` paints the rule in the alert colour for NEEDS YOU), else null.
export function cardBottom(state, agent, label) {
  if ((state !== "working" && state !== "needs_you") || !agent) return null;
  return { kind: "terminal", address: agent.address, label, alert: state === "needs_you" };
}

// The bottom field's rule colour: NEEDS YOU alert, the coordinator frame, other terminals working, the tree section.
export function ruleColor(b) {
  if (b?.kind !== "terminal") return ROLE.section;
  if (b.alert) return ROLE.alert;
  return String(b.label).startsWith("COORDINATOR") ? ROLE.frame : ROLE.working;
}

const machineName =(m) => (m == null || m === "" || m === "local" ? null : String(m));

// { machine, id } for an address, `machine` used when the address carries none
export function parseAddress(address, machine = null) {
  const s = String(address ?? "");
  const i = s.lastIndexOf("/");
  if (i < 0) return { machine: machineName(machine), id: s };
  return { machine: machineName(s.slice(0, i)), id: s.slice(i + 1) };
}

const FINISHED = new Set(["finished", "exited", "dead", "killed", "gone"]);

// Whether the terminal at `address` (on `machine` when the address is bare) is "live", "gone" or "unknown",
// judged from `list` (herdr's terminals(): { address, machine?, state? }). It is "gone" only when it is positively
// known gone: listed as finished, or missing while other terminals of its machine are listed. A machine with no
// terminals listed (offline, still loading) or an empty list says "unknown".
export function terminalState(address, list, machine = null) {
  if (!address || !Array.isArray(list) || !list.length) return "unknown";
  const want = parseAddress(address, machine);
  let machineSeen = false;
  for (const t of list) {
    const got = parseAddress(t?.address, t?.machine ?? null);
    if (got.machine !== want.machine) continue;
    machineSeen = true;
    if (got.id === want.id) return FINISHED.has(String(t?.state ?? "")) ? "gone" : "live";
  }
  return machineSeen ? "gone" : "unknown";
}
