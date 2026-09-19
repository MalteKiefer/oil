# verbrauch

[![CI](https://github.com/MalteKiefer/oil/actions/workflows/ci.yml/badge.svg)](https://github.com/MalteKiefer/oil/actions/workflows/ci.yml)
[![Lint](https://github.com/MalteKiefer/oil/actions/workflows/lint.yml/badge.svg)](https://github.com/MalteKiefer/oil/actions/workflows/lint.yml)
[![Security](https://github.com/MalteKiefer/oil/actions/workflows/security.yml/badge.svg)](https://github.com/MalteKiefer/oil/actions/workflows/security.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Kommandozeilen-Tool zur Erfassung von Verbrauchszählerständen (Heizöl, Strom,
und beliebige weitere Verbrauchsarten). Berechnet den Verbrauch automatisch
aus aufeinanderfolgenden Zählerständen, zeigt ihn tabellarisch und als
ASCII-Diagramm über 7 Tage / 30 Tage / 6 Monate / 12 Monate an, und liefert
eine Verbrauchs-, Kosten- und Tankreichweiten-Prognose.

## Features

- Beliebige Verbrauchsarten (Öl, Strom, ...), nicht auf Heizöl beschränkt
- Automatische Verbrauchsberechnung aus Zählerstand-Differenzen
- Bericht als Tabelle + ASCII-Balkendiagramm für 7d/30d/6m/12m
- Prognose: Verbrauchstrend, Kostenhochrechnung, Tage bis Tank leer
- Heizmodus pro Eintrag (aus / nur Warmwasser / Warmwasser + Heizen)
- SQLite-Datenbank (WAL-Modus), kein Server, keine Cloud
- Single-Binary, kein CGO, läuft auf Windows/Linux/macOS (amd64 + arm64)
- `--json` für maschinenlesbare Ausgabe

## Installation

Binaries für Windows, Linux und macOS (amd64 + arm64) liegen bei jedem
[Release](https://github.com/MalteKiefer/oil/releases).

Oder aus dem Quellcode bauen (Go 1.26+):

```bash
go install github.com/maltekiefer/verbrauch/cmd/verbrauch@latest
```

## Verwendung

```bash
# Verbrauchsart anlegen
verbrauch type add oel --unit L --price 0.95 --tank-size 3000
verbrauch type list

# Zählerstand erfassen
verbrauch add --type oel --value 45231.5 --heating both

# Tankbefüllung erfassen
verbrauch refill --type oel --amount 2500

# Bericht: Tabelle + Diagramm
verbrauch report --type oel --period 30d

# Prognose: Verbrauch, Kosten, Tankreichweite
verbrauch forecast --type oel --horizon 30d

# Kurzübersicht
verbrauch status --type oel
```

Jeder Befehl unterstützt `--json` für maschinenlesbare Ausgabe und `--db
<pfad>` zum Überschreiben des Standard-Datenbankpfads (sonst
`$XDG_CONFIG_HOME`/`os.UserConfigDir()`, überschreibbar auch über die
Umgebungsvariable `VERBRAUCH_DB`).

## Entwicklung

```bash
go build ./...
go vet ./...
go test ./...
```

Design-Spezifikation und Implementierungsplan liegen unter
[`docs/superpowers/`](docs/superpowers/).

## Lizenz

[MIT](LICENSE)
