# Design: LCARS touch sizes everywhere, rollout on remote1

## Sizes in the frame (`lcars/main.js`, `layouts/lcars.json`)

The rules of "Sizes" in `docs/lcars-colors.md` (TEXT_MIN 14, TEXT_BODY 16, TOUCH 44) now cover the whole
layout, not only the Bridge, the tab bar and Striker's board.

### Sidebar (`side()`)

| Element | Today | New |
|---|---|---|
| Space row | 26 px high, index 12 px, name 14 px | `TOUCH` high, index `TEXT_MIN`, name `TEXT_BODY`, state oval 22×12 |
| Machine block label | 11 px | `TEXT_MIN`; a long name ("THIS COMPUTER") wraps onto two lines inside the 72 px block |
| `+ NEW SPACE` slab | 30 px | `TOUCH` (pill radius 22) |
| Section headings FLEET, CREW | 13 px, 18 px high | `TEXT_MIN`, 20 px high |
| STRIKER slab | 40 px, count 12 px | `TOUCH`, count `TEXT_MIN`, word 16 px |
| Crew row | 40 px, title 13 px, line 11 px | `TOUCH`, title `TEXT_MIN`, line `TEXT_MIN`; the two lines fit in 44 px at 14 px |

The whole sidebar scrolls vertically (one scroll area: spaces, FLEET, CREW). The grey filler at its foot
keeps running into the bottom elbow when there is room left; with too little room it keeps its 10 px
minimum at the end of the scrolled content.

The dead Shepherdr sidebar code (`shepherdr()` method; the sidebar stopped showing it) is removed; the
status arm's SHEPHERDR count stays.

### Machines popup (`machines()`)

Rows `TOUCH` high; the popup height becomes `12 + 30 + (GAP + TOUCH) × (n + 1)` (padding, title, rows).

### Top frame (`top()`)

The block stays 64 px; nothing in it is clickable except the right-click menu (the whole block). The elbow
label (machine) and the stardate go from 13 px to `TEXT_MIN`.

### Bottom frame (`bottom()`)

The layout's `bottom` block grows from 44 to 60 px: a `TOUCH`-high status row over the 16 px arm. The
status texts go from 12 px to `TEXT_MIN`. STRIKER, SHEPHERDR and the palette/clock are clickable cells as
high as the row (`TOUCH`) with horizontal padding so each is at least `TOUCH` wide. The gear gets a
`TOUCH`×`TOUCH` hit box around its 18 px icon. The elbow (MENU) is 210×44 and needs nothing more; its
text goes to `TEXT_MIN`.

The terminal area loses 16 px of height.

### Bridge (`lcars/bridge.js`)

`ROW` 56 → 60 so a working card's title, status line and step bar are not clipped. OVERVIEW rows stay 64.

### Docs

`docs/lcars-colors.md`, "Sizes": the constants apply everywhere; drop the sentence that the sidebar and the
frame keep their older, smaller text; PLAN rows 60 px; bottom block 60 px.

## Rollout on remote1 (no code unless noted)

1. **A user `awayteam` for the agents, instead of root.** Today leads run as root (`sudo -i … claude`) and
   `coord.rootGit` runs `sudo -n -i git …` / `cat …`, which would need passwordless root for the owner. Instead:
   - User and group `awayteam` (home `/home/awayteam`, bash), member of the extra groups the leads need for
     their work (the agent user's extra group rights, accepted by the person), not of `sudo`.
   - The owner's sudo rule (`<owner> ALL=(awayteam) NOPASSWD: ALL`, installed by `striker/setup/awayteam.sh`):
     the owner becomes awayteam without a password; awayteam cannot become root.
   - Repos `/srv/alpha`, `/srv/beta`, `/srv/gamma`, `/srv/delta` and their worktrees: ACL
     `u:awayteam:rwX` recursively plus the same default ACL (owners stay). awayteam's git config
     `safe.directory = *` (the repos belong to root or the owner).
   - Status files move to `/var/lib/striker/status` (owner: the owner, group awayteam, mode 2775, default ACL
     group rwx); the files under `/home/you/.striker/status` are moved there; `fleet.json` `status_dir`
     follows.
   - Claude Code installed for awayteam (`~/.local/bin/claude`), logged in once by the person; root's
     `~/.claude/projects` copied to awayteam so the leads keep their memory.
   - Striker: a fleet entry's `user` other than root and empty runs Claude as
     `sudo -u <user> -i sh -c 'cd <dir> && exec "$HOME/.local/bin/claude"'` and git/cat as
     `sudo -n -u <user> -i …`; `root` stays as it was. `fleet.json` sets `"user": "awayteam"` for all four
     repos. Running root leads keep running; they are restarted as awayteam when the person says so.
   - One script, `striker/setup/awayteam.sh`, run once by the person as root on remote1, does the system part
     (idempotent; `visudo -c` before the sudo rule is installed).
2. **openspec CLI on remote1.** There is no standalone binary; it is the npm package
   `@fission-ai/openspec` and needs Node ≥ 20. Install Node (distribution package or NodeSource), then
   `npm install -g @fission-ai/openspec` as root (the leads and coordinators run as root), then
   `openspec config set telemetry.enabled false` as root.
3. **First live coordinator run.** On a small real change of a fleet repo the person picks:
   `striker start-coordinator <repo> <change>`; check that the Bridge shows its card, Striker's board its line,
   and that a question lands in `needs_you` and reaches the person.
4. **Lead tabs named `claude`.** Existing lead tabs on remote1 (`beta / Claude Code`, `gamma / 1`,
   `delta / 1`) are renamed to `claude` with `herdr-app do tab.rename`, matching what `start-lead` makes.

## Testing

`node lcars/bridge.check.mjs` stays green; `herdr-app check lcars` passes; screenshots (`herdr-app shot`)
of the sidebar with several spaces (scrolling), the bottom frame, the machines popup and a PLAN card with a
step bar.
