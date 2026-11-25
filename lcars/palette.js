// The LCARS palette and what each colour means. docs/lcars-colors.md has the rules.
// C holds the raw hues; decoration (elbows, arms, filler blocks) may use any of them.
// ROLE and STATUS hold the meanings; anything that tells the person something uses those.

export const C = {
  black: "#000000", orange: "#ff9900", peach: "#ff9966", gold: "#ffcc66", tan: "#ffcc99", lilac: "#cc99cc",
  violet: "#9999ff", purple: "#aa77cc", blue: "#99ccff", red: "#ff5544", gray: "#666688", dark: "#333344",
};

export const ROLE = {
  bg: C.black, // background and the gaps between blocks
  ink: C.black, // text set on a block of colour
  frame: C.orange, // the elbows, the title bar, the space name
  active: C.orange, // what is selected: the active space or tab, the focused agent, the current phase
  alert: C.red, // waits on the person: blocked, needs you, asks you
  done: C.gold, // finished: done, ready to merge
  working: C.blue, // busy: working, progress made
  idle: C.gray, // standing by: nothing to report, so it reads as neutral
  hover: C.tan, // a grey button under the pointer lights up
  text: C.tan, // content text: names, tasks, rows
  section: C.peach, // a section heading: text on black, never a filled block
  label: C.peach, // small field labels on Striker's board
  neutral: C.gray, // inactive, offline, zero, plain buttons
  track: C.dark, // the empty part of a phase row or progress bar
};

export const STATUS = { working: ROLE.working, blocked: ROLE.alert, done: ROLE.done, idle: ROLE.idle };
export const WORD = { working: "WORKING", blocked: "NEEDS YOU", done: "DONE", idle: "STANDING BY" };

// sizes for touch and legibility (docs/lcars-colors.md, "Sizes"): no text under TEXT_MIN, rows and body text
// TEXT_BODY, and every clickable thing at least TOUCH high (and TOUCH wide when it is small)
export const TEXT_MIN = 14;
export const TEXT_BODY = 16;
export const TOUCH = 44;

// spaces tell themselves apart by colour: the purple family, which nothing else uses
export const SPACE_COLORS = [C.lilac, C.violet, C.purple];
