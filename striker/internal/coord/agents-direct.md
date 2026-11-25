2. Starte jeden Implementer und jeden Reviewer mit
   `striker dispatch <rolle> <task> --plan {change}`
   (Rolle z. B. `implementer`, `spec-reviewer`, `quality-reviewer`; `<task>` ist die Task-ID wie `2.3`). Der Befehl gibt die Adresse des neuen Terminals aus.
3. Warte mit `hc wait <adresse>`, dann lies das Ergebnis mit `hc read <adresse>`. Warte nie mit `sleep`. Blockiert ein Task-Agent, entscheidest du oder trägst die Frage in dein `needs_you` ein; Task-Agents eskalieren nie an dir vorbei.
