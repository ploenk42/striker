// Striker's prompt under his board is the layout block "striker-prompt", which can only name a fixed terminal.
// When Striker is restarted his terminal changes, and Herdr drops a block whose terminal is gone; the sidebar then
// points the block at the new terminal, or puts it back under the board (withPromptTerminal, applied with
// layout.set). No gpui imports, so node can check it: bridge.check.mjs.

export const PROMPT_ID = "striker-prompt";
export const BOARD_ID = "striker-board"; // the column that holds the board and, below it, the prompt

// the block as layouts/lcars.json has it: 300 px (about 12 rows at font 14), so Claude's status, a prompt of a few
// lines and its footer fit; at 160 px Claude got 7 rows and showed one line of the prompt
export const promptBlock = (address) => ({
  id: PROMPT_ID, type: "block", content: { terminal: address }, size: { px: 300 }, min: 300,
  background: "#000000", border: { width: 2, color: "#ff9900" }, radius: 10, padding: 4,
});

const hasPrompt = (node) => !!node && typeof node === "object" &&
  (Array.isArray(node) ? node.some(hasPrompt) : node.id === PROMPT_ID || Object.values(node).some(hasPrompt));

// The layout tree with the block PROMPT_ID showing terminal `address` (put back as the last child of BOARD_ID when
// Herdr dropped it), or null when nothing needs to change or there is no place for it. The tree given is not changed.
export function withPromptTerminal(tree, address) {
  if (!tree || typeof tree !== "object" || !address) return null;
  const missing = !hasPrompt(tree);
  let changed = false;
  const walk = (node) => {
    if (!node || typeof node !== "object") return node;
    if (Array.isArray(node)) return node.map(walk);
    if (node.id === PROMPT_ID && node.type === "block") {
      if (node.content?.terminal === address) return node;
      changed = true;
      return { ...node, content: { terminal: address } };
    }
    const out = {};
    for (const [k, v] of Object.entries(node)) out[k] = v && typeof v === "object" ? walk(v) : v;
    if (missing && node.id === BOARD_ID && Array.isArray(node.children)) {
      out.children = [...out.children, promptBlock(address)];
      changed = true;
    }
    return out;
  };
  const next = walk(tree);
  return changed ? next : null;
}
