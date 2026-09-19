# verbrauch — CLI zur Erfassung von Öl-/Strom-/Zählerständen

Datum: 2026-09-19
Status: Approved, bereit für Implementierungsplan

## Zweck

Kommandozeilen-Tool, das tägliche Zählerstände (z.B. Heizöl, Strom) erfasst,
daraus automatisch den Verbrauch pro Zeitraum berechnet, tabellarisch und als
ASCII-Chart darstellt, und eine Verbrauchs-/Kosten-/Reichweitenprognose liefert.
Nicht auf Öl beschränkt — Verbrauchsarten sind generisch konfigurierbar.

## Nicht-Ziele

- Keine GUI, kein Web-Interface.
- Keine automatische Zählerablesung (IoT/Sensoren) — Eingabe ist manuell.
- Keine Mehrbenutzer-/Mehrhaushalt-Verwaltung in v1.

## Datenmodell (SQLite)

```sql
CREATE TABLE consumption_types (
  id            INTEGER PRIMARY KEY,
  name          TEXT UNIQUE NOT NULL,   -- z.B. "oel", "strom"
  unit          TEXT NOT NULL,          -- z.B. "L", "kWh"
  price_per_unit REAL,                  -- nullable, Währung/Einheit
  tank_size     REAL                    -- nullable, nur für Typen mit Reichweitenberechnung
);

CREATE TABLE readings (
  id            INTEGER PRIMARY KEY,
  type_id       INTEGER NOT NULL REFERENCES consumption_types(id),
  reading_date  TEXT NOT NULL,          -- YYYY-MM-DD
  counter_value REAL NOT NULL,          -- kumulativer Zählerstand
  heating_mode  TEXT,                   -- 'off' | 'water_only' | 'water_and_heat', nullable
  created_at    TEXT NOT NULL,
  UNIQUE(type_id, reading_date)
);

CREATE TABLE refills (
  id          INTEGER PRIMARY KEY,
  type_id     INTEGER NOT NULL REFERENCES consumption_types(id),
  refill_date TEXT NOT NULL,
  amount      REAL NOT NULL,
  created_at  TEXT NOT NULL
);
```

Verbrauch zwischen zwei Messungen: `diff = counter_b - counter_a`, verteilt
über die Anzahl Tage zwischen den beiden Daten (Durchschnitt/Tag). Erste
Messung eines Typs hat keinen Vorgänger und liefert keinen Verbrauchswert.

Tankfüllstand (nur wenn `tank_size` gesetzt und mindestens eine `refill`
existiert): `restmenge = letzte_refill.amount - Summe(diffs seit refill.date)`.

Negative Diffs (Zählerwechsel, Tippfehler) werden beim Einlesen erkannt, als
Warnung auf stderr ausgegeben, und von Aggregation/Trend ausgeschlossen.

## CLI-Befehle (cobra)

```
verbrauch type add --name oel --unit L [--price 0.95] [--tank-size 3000]
verbrauch type list

verbrauch add --type oel --value 45231.5 [--date 2026-09-19] [--heating off|water|both]
  # --date Default: heute
  # --heating Default: kein Wert (null)

verbrauch refill --type oel --amount 2500 [--date 2026-09-19]

verbrauch report --type oel --period 7d|30d|6m|12m [--json]
  # Tabelle: Datum, Zählerstand, Diff, Heizmodus
  # Chart: ASCII-Balken (Unicode-Blocks)

verbrauch forecast --type oel [--horizon 7d|30d] [--json]

verbrauch status --type oel
  # Kurzübersicht: Füllstand, Reichweite, letzter Verbrauch/Tag
```

Exit-Codes: 0 Erfolg, 1 allgemeiner Fehler, 2 falsche Benutzung.
Nutzdaten → stdout, Diagnostik/Fehler → stderr.
`--json` für maschinenlesbare Ausgabe, menschliche Ausgabe (Tabelle+Chart,
Farben) nur wenn stdout ein TTY ist; `NO_COLOR` wird respektiert.
Konfigurationsreihenfolge: Flags > Umgebungsvariablen > Konfigdatei > Defaults.
`SIGINT`/`SIGTERM` über `signal.NotifyContext` abfangen, DB sauber schließen.

## Report-Aggregation

| Periode | Granularität        |
|---------|----------------------|
| 7d      | Tage                 |
| 30d     | Tage                 |
| 6m      | Wochen (~26 Balken)  |
| 12m     | Monate (12 Balken)   |

## Forecast-Logik

- Durchschnittsverbrauch/Tag aus den letzten 30 Tagen (weniger falls nicht
  genug Daten vorhanden; Fehler "zu wenig Daten" bei < 2 gültigen Messpunkten).
- Verbrauchsprognose = Durchschnitt/Tag × Horizont-Tage.
- Kostenprognose nur wenn `price_per_unit` für den Typ gesetzt ist.
- Tage-bis-leer nur wenn `tank_size` gesetzt und mindestens eine `refill`
  existiert: `restmenge / durchschnitt_pro_tag`.

## Tech-Stack

- Go, Minimum-Version `go 1.26` (stabil; 1.27 ist zum Zeitpunkt der Spec noch
  Pre-Release) in `go.mod`.
- SQLite-Treiber: `modernc.org/sqlite` (pure Go, kein CGO, einfaches
  Cross-Compiling für Windows/Linux/macOS als Single-Binary).
- CLI-Framework: `spf13/cobra` (Subcommands, automatische Hilfetexte).
- Dependency-Versionen werden beim Scaffolding per `go get <modul>@latest`
  aufgelöst, nicht in dieser Spec hartkodiert.
- Chart-Rendering: keine externe Library, ASCII-Balken selbst gerendert
  (Unicode-Blockzeichen).
- Migrationen: SQL-Dateien per `//go:embed`, angewendet über
  `PRAGMA user_version` als Stand-Marker (kein Migrations-Framework, bei
  3 Tabellen overkill).
- DB-Pfad: `os.UserConfigDir()/verbrauch/verbrauch.db`, überschreibbar mit
  `--db`.
- Beim Öffnen der DB: `PRAGMA journal_mode = WAL`, `PRAGMA busy_timeout = 5000`,
  `PRAGMA foreign_keys = ON` (gilt pro Connection), `PRAGMA synchronous = NORMAL`.
- Schreibverbindung: `SetMaxOpenConns(1)` (SQLite erlaubt nur einen Writer).

## Projektstruktur

```
cmd/verbrauch/main.go   -- Entry point, signal.NotifyContext
internal/store/         -- DB-Öffnen, Migrationen, Queries (database/sql)
internal/cli/           -- Cobra-Commands
internal/report/        -- Aggregation, Chart-Rendering
internal/forecast/      -- Trend-/Kosten-/Reichweiten-Berechnung
```

## Testing

- Tabellengetriebene Tests, `t.Run` für Subtests.
- `internal/store`: echtes SQLite in `t.TempDir()`, keine Mocks — Migrationen,
  CRUD, UNIQUE-Constraint-Verletzung (Doppeleintrag selber Tag).
- `internal/report`: Aggregationslogik mit festen Beispiel-Zeitreihen
  (inkl. Lücken, negativer Diff → übersprungen).
- `internal/forecast`: Trend-/Kosten-/Reichweitenberechnung mit bekannten
  Werten, Edge Case "zu wenig Daten".
- `internal/cli`: dünn halten, keine eigene Logik testen die schon in
  store/report/forecast getestet ist.
- `go vet`, `staticcheck`, `golangci-lint` müssen sauber durchlaufen.

## Offene Punkte / bekannte Einschränkungen

- Keine Korrektur/Löschung fehlerhafter Einträge in v1 spezifiziert (nur
  Warnung bei negativem Diff) — bei Bedarf später `verbrauch edit`/`delete`.
- `heating_mode` ist optional und typunabhängig; bei Strom-Einträgen ohne
  Heizungsbezug bleibt das Feld leer.
