# Striker

**Dokumentation:** https://ploenk42.github.io/striker/

Der Leitstand: ein Claude Code, mit dem der Mensch als Einziges spricht. Striker kennt die Repos aus
`fleet.json`, liest den Stand ihrer Leads und gibt Aufträge an sie weiter. Die Rolle steht in
`CLAUDE.md`.

| Datei | Zweck |
|---|---|
| `CLAUDE.md` | Strikers Rolle und Regeln |
| `fleet.json` | die Repos: Rechner, Pfad, Benutzer, Herdr-Space; wo die Statusdateien liegen |
| `briefs/lead.md` | die Einweisung eines Leads, mit dem Format seiner Statusdatei |
| `briefs/coordinator.md` | die Einweisung eines Koordinators |
| `internal/lead/coordinators-*.md`, `internal/coord/agents-*.md` | die zwei Fassungen (direkt, vermittelt) von `{coordinators}` und `{agents}` in den Einweisungen; ins Programm eingebaut |
| `cmd/striker`, `internal/` | das Go-Programm `striker`: `status`, `send`, `start-lead`, `serve` |
| `launchd/` | Vorlage des macOS-LaunchAgents, den `install.sh` einrichtet |
| `setup/awayteam.sh` | als root auf dem entfernten Rechner (wiederholbar), `sudo bash awayteam.sh <owner> <repo-pfad>...`: Benutzer `awayteam` für alle Agents, sudo nur zu ihm; installiert `setup/awayteam-settings.json` als dessen Claude-Einstellungen und `setup/statusline.sh` als dessen Statuszeile (mit `hc put` alle drei Dateien hinüber, nicht über eine PowerShell-Pipe: die setzt BOM und CRLF) |
| `setup/statusline.sh` | Claudes Statuszeile auf Linux-Rechnern: legt Modell und Tokens je Terminal in `<agents_dir>/<terminal>.json` ab, für Strikers Crew |

`user` in `fleet.json`: leer ist der Benutzer, als den `hc` sich anmeldet; `root` läuft über `sudo -i`; jeder
andere Name (auf remote1 `awayteam`) über `sudo -u <name>`, nie als root.

`striker serve` liefert den Stand auf `127.0.0.1:7522/status` (das LCARS-Board liest ihn dort). Es startet
beim Anmelden: unter macOS als LaunchAgent `io.github.ploenk42.striker`, unter Windows als Aufgabe `Striker`;
beide richtet der Installer im Repo-Stamm ein. `STRIKER_HOME` verweist auf dieses Verzeichnis
(Standard `<Benutzerordner>/striker/striker`).

Ein Lead trägt das hc-Label `striker.lead=<repo>` und schreibt `<status_dir>/<repo>.json` auf seinem Rechner.

Modell und Tokens eines Agents kommen aus Claudes Statuszeile: `striker statusline` (auf Strikers Rechner, vom
Installer mit `striker install-statusline` eingetragen, falls noch keine Statuszeile gesetzt ist) oder
`setup/statusline.sh` (awayteam auf remote1) schreiben `<agents_dir>/<HC_TERMINAL_ID>.json`; `agents_dir` in
`fleet.json` je Rechner, sonst `~/.striker/agents`. Striker startet Agents unter einem anderen Benutzer mit
`sudo --preserve-env=HC_TERMINAL_ID`, damit die Terminal-ID ankommt.

Agents eines Repos mit `user` (auf remote1 `awayteam`) haben keinen Zugang zu `hc` und können darum weder
`striker` noch `hc` aufrufen; `striker serve` vermittelt für sie. Alle 10 s liest es auf jedem solchen Rechner die
Statusdateien der letzten 24 Stunden und führt deren neue `requests` aus: `start-coordinator` (vom Lead),
`dispatch` und `send` (vom Koordinator). Task-Agents startet es als der Benutzer aus `fleet.json` im Worktree des
Changes, unter dem Koordinator; es antwortet dem Anfragenden mit einer `[Striker]`-Nachricht und schickt dem
Koordinator die letzte Antwort eines Task-Agents, sobald der fertig ist oder auf eine Eingabe wartet. Striker
schreibt nie in die Statusdateien; was er getan hat, steht in `~/.striker/relay.json` (Schlüssel `<rechner>|<datei>|<id>`).
Die Einweisungen bekommen je nach Repo die direkte oder die vermittelte Fassung (`{coordinators}`, `{agents}`).

Leads und Koordinatoren startet Striker mit `claude --remote-control '<name>'` (`lead <repo>`, `coord <repo>
<change>`), so tauchen sie nach jedem Neustart wieder in claude.ai/code und der App auf. Ein Benutzer, der
Remote Control noch nie bestätigt hat, bekommt beim ersten Mal einen Dialog im Terminal (mit Enter bestätigen).
