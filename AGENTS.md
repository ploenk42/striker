# herdr-mods

Herdr-Mods des Menschen: LCARS-Layout und -Mods (`lcars/`), Striker als Go-Programm (`striker/`).

## Arbeitsweise: OpenSpec + Superpowers

Der Speicher ist OpenSpec, der Prozess sind die Superpowers-Skills (Brainstorming, Planen, TDD,
Subagents, Reviews). OpenSpecs eigene Skills (propose/apply/archive) werden nicht benutzt; die
`openspec`-CLI dient nur für `validate`, `list` und `archive`.

Layout:

```
openspec/
  specs/<capability>/spec.md            lebende Anforderungen (SHALL + Szenarien)
  changes/<name>/
    .openspec.yaml                      Metadaten (x-depends_on, x-branch)
    proposal.md                         Warum, Was ändert sich, Capabilities, Impact
    design.md                           das Design
    specs/<capability>/spec.md          Spec-Deltas (ADDED / MODIFIED / REMOVED)
    tasks.md                            Checkliste = Stand des Changes
    plan.md                             der Superpowers-Plan (Schritte, Dateien, Tests)
  changes/archive/<YYYY-MM-DD>-<name>/  fertige Changes
```

Zuordnung:

1. **Brainstorming** schreibt `proposal.md` (kurz) und `design.md` des Changes, dazu Spec-Deltas für
   Capabilities, deren Verhalten sich ändert. Nicht nach `docs/superpowers/` schreiben.
2. **Planen** schreibt `tasks.md` (eine Zeile pro Task) und `plan.md` (der detaillierte Plan, je
   Task `### Task <id>: <Titel>`).
3. **Umsetzen** (subagent-driven oder executing-plans): `tasks.md` wird im selben Commit
   abgehakt wie die letzte Arbeit des Tasks. Coordinator und `striker dispatch` lesen die Tasks aus
   `tasks.md`.
4. **Abschluss:** `openspec validate <name> --strict`, nach dem Merge auf dem Default-Branch
   `openspec archive <name> --yes`.

Task-Zeilen: nur Top-Level-Zeilen in `tasks.md` zählen, `- [ ] <id> <Titel>`, optional mit
Suffix `(#<issue>; after <id>, <id>)`. Ohne `after` hängt ein Task vom vorherigen ab. `striker issues`
schreibt die `#n`.

Metadaten eines Changes stehen in dessen `.openspec.yaml` mit `x-`-Präfix: `x-depends_on: [<change>]`
(Changes, die zuerst archiviert sein müssen) und `x-branch: <name>` (Standard: der Change-Name).
Ein Change wird mit `proposal.md`, `design.md`, `tasks.md` und `plan.md` auf dem Default-Branch
angelegt; Arbeit und Haken liegen auf `x-branch`, dessen `tasks.md` gilt.

Koordinator: pro Change einer, gestartet mit `striker start-coordinator <repo> <change>` (Tab
`coord <change>`). Er berichtet über `<status_dir>/<repo>--<change>.json` an Striker und trägt alles,
was er nicht entscheiden kann, in sein `needs_you` ein.

Shell-Befehle: genau ein einfacher Befehl pro Aufruf, absolute Pfade. Keine `$(...)`, Backticks,
Heredocs, Variablen, Schleifen, Subshells, `cd … &&`-Ketten, Verkettung mit `;`, `&&`, `||` oder
Pipes, keine Globs, keine Umleitungen (auch nicht nach `/dev/null`). Dateien mit Read, Edit und
Write lesen und ändern.
