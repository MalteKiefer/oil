# verbrauch CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-binary Go CLI (`verbrauch`) that records daily meter readings for arbitrary consumption types (oil, electricity, ...), computes consumption automatically, reports it as table + ASCII chart over 7d/30d/6m/12m, and forecasts future consumption, cost, and tank range.

**Architecture:** Layered internal packages — `store` (SQLite persistence, no business logic) → `report` (pure diff/aggregation/chart logic, no DB or CLI dependency) → `forecast` (pure trend/cost/range math, depends only on `report`'s types) → `cli` (cobra commands wiring the layers together) → `cmd/verbrauch` (process entry point: signal handling, exit codes).

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure-Go SQLite driver), `spf13/cobra` (CLI framework), standard library only otherwise.

**Spec:** [docs/superpowers/specs/2026-09-19-verbrauch-cli-design.md](../specs/2026-09-19-verbrauch-cli-design.md)

## Global Constraints

- Go version floor: `go 1.26` in `go.mod` (Go 1.27 is still pre-release at spec time — do not use it).
- Dependency versions are resolved live via `go get <module>@latest` at scaffold time, never hardcoded from memory.
- SQLite driver: `modernc.org/sqlite` (pure Go, no CGO). Driver name registered for `sql.Open` is `"sqlite"`.
- On every `store.Open`: set `PRAGMA journal_mode = WAL`, `PRAGMA busy_timeout = 5000`, `PRAGMA foreign_keys = ON`, `PRAGMA synchronous = NORMAL`, and `SetMaxOpenConns(1)` (SQLite allows a single writer; this tool has no read-heavy concurrency need, so one shared connection is used for both reads and writes).
- All SQL uses `database/sql` prepared statements with `?` placeholders. Never string-concatenate SQL.
- All errors wrapped with `fmt.Errorf("...: %w", err)`; sentinel errors checked with `errors.Is`/`errors.As`.
- Nutzdaten (command output) → stdout. Diagnostics, warnings, errors → stderr.
- Exit codes: 0 success, 1 general error. **Documented deviation:** exit code 2 ("wrong usage") is only used for the subset of usage mistakes the CLI layer explicitly detects and wraps in `cli.UsageError` (invalid `--period`/`--heating`/`--horizon`/date values, unknown `--type` name). Cobra's own internal flag-parsing errors (missing required flag, unknown flag) are not re-classified and fall back to exit code 1, because cobra does not expose a stable typed error for that case and string-matching cobra's internal messages would be fragile.
- `--json` flag (persistent, root level) switches every command's output to machine-readable JSON. Human-readable output (tables via `text/tabwriter`, ASCII chart) is the default. No color output and no TTY detection in v1 — the design has no color scheme, so this is out of scope (YAGNI).
- DB path resolution order: `--db` flag > `VERBRAUCH_DB` env var > `os.UserConfigDir()/verbrauch/verbrauch.db`. No config file in v1 — there are only two global settings (`--db`, `--json`), both already covered by flags/env, so a config file layer is unneeded (YAGNI); this is a scope decision versus the spec's generic "Konfigdatei" mention, noted here explicitly.
- `SIGINT`/`SIGTERM` handled via `signal.NotifyContext` in `main.go`; context threaded into every `database/sql` call via `...Context` variants.
- Tests are table-driven with `t.Run` subtests. `internal/store` tests use real SQLite in `t.TempDir()`, never mocks. `internal/report` and `internal/forecast` tests use plain in-memory fixtures (pure functions, no DB).
- Every exported function/type used by a later task is defined with its final name and signature the first time it appears below — later tasks must match these exactly.
- Module path: `github.com/maltekiefer/verbrauch`.

---

## Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `internal/store/migrations/0001_init.sql`
- Create: `cmd/verbrauch/main.go`

**Interfaces:**
- Produces: module `github.com/maltekiefer/verbrauch`, buildable with `go build ./...`.

- [ ] **Step 1: Initialize the module**

Run:
```bash
go mod init github.com/maltekiefer/verbrauch
```

Edit the generated `go.mod` so the `go` directive reads:
```
go 1.26
```

- [ ] **Step 2: Add dependencies at their latest versions**

Run:
```bash
go get github.com/spf13/cobra@latest
go get modernc.org/sqlite@latest
```

This writes the resolved version numbers into `go.mod`/`go.sum` — do not hand-edit version strings.

- [ ] **Step 3: Create `.gitignore`**

```
/verbrauch
/verbrauch.exe
*.db
*.db-wal
*.db-shm
```

- [ ] **Step 4: Create the initial schema migration**

`internal/store/migrations/0001_init.sql`:
```sql
CREATE TABLE consumption_types (
  id             INTEGER PRIMARY KEY,
  name           TEXT UNIQUE NOT NULL,
  unit           TEXT NOT NULL,
  price_per_unit REAL,
  tank_size      REAL
);

CREATE TABLE readings (
  id            INTEGER PRIMARY KEY,
  type_id       INTEGER NOT NULL REFERENCES consumption_types(id),
  reading_date  TEXT NOT NULL,
  counter_value REAL NOT NULL,
  heating_mode  TEXT,
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

- [ ] **Step 5: Create a minimal buildable entry point**

`cmd/verbrauch/main.go`:
```go
package main

import "fmt"

func main() {
	fmt.Println("verbrauch: CLI zur Erfassung von Zählerständen (Öl, Strom, ...)")
}
```

(This is a genuine, working first version — it gets replaced with full wiring in Task 14, not a placeholder.)

- [ ] **Step 6: Verify it builds and runs**

Run:
```bash
go build ./...
go run ./cmd/verbrauch
```
Expected: builds with no errors, prints the line from Step 5.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum .gitignore internal/store/migrations/0001_init.sql cmd/verbrauch/main.go
git commit -m "chore: scaffold verbrauch module, deps, initial schema"
```

---

## Task 2: Store — Open, Migrations, Pragmas

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Produces:
  - `type DB struct { *sql.DB }`
  - `func Open(ctx context.Context, path string) (*DB, error)`
  - (internal) `func migrate(ctx context.Context, db *sql.DB) error`

- [ ] **Step 1: Write the failing test**

`internal/store/store_test.go`:
```go
package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func TestOpen_CreatesSchemaAndPragmas(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sub", "verbrauch.db")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 1 {
		t.Errorf("user_version = %d, want 1", version)
	}

	tables := []string{"consumption_types", "readings", "refills"}
	for _, table := range tables {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestOpen_ReopenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	db1, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first handle: %v", err)
	}

	db2, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() { _ = db2.Close() }()

	var version int
	if err := db2.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 1 {
		t.Errorf("user_version after reopen = %d, want 1", version)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/... -run TestOpen -v`
Expected: FAIL — `package store: no Go files` / `undefined: store.Open`.

- [ ] **Step 3: Implement `store.Open` and migrations**

`internal/store/store.go`:
```go
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps *sql.DB with the verbrauch schema and pragmas already applied.
type DB struct {
	*sql.DB
}

// Open opens (creating if necessary) the SQLite database at path, applies
// the required pragmas, and runs any pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = NORMAL",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.ExecContext(ctx, p); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	if err := migrate(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return &DB{sqlDB}, nil
}

// migrate applies every migration file under migrations/ whose ordinal
// (by filename sort order) is greater than the database's current
// PRAGMA user_version, in order, each in its own transaction.
func migrate(ctx context.Context, db *sql.DB) error {
	var currentVersion int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&currentVersion); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for i, entry := range entries {
		targetVersion := i + 1
		if targetVersion <= currentVersion {
			continue
		}

		contents, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", entry.Name(), err)
		}
		defer func() { _ = tx.Rollback() }()

		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		// PRAGMA user_version does not support bound parameters; targetVersion
		// is an internal loop counter, never user input, so this is safe.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", targetVersion)); err != nil {
			return fmt.Errorf("set user_version to %d: %w", targetVersion, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/... -run TestOpen -v`
Expected: PASS for both `TestOpen_CreatesSchemaAndPragmas` and `TestOpen_ReopenIsIdempotent`.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: add store.Open with pragmas and embedded migrations"
```

---

## Task 3: Store — Consumption Types

**Files:**
- Create: `internal/store/types.go`
- Test: `internal/store/types_test.go`

**Interfaces:**
- Consumes: `store.Open` (Task 2)
- Produces:
  - `type ConsumptionType struct { ID int64; Name string; Unit string; PricePerUnit sql.NullFloat64; TankSize sql.NullFloat64 }`
  - `var ErrTypeNotFound error`
  - `var ErrDuplicateType error`
  - `func (db *DB) CreateType(ctx context.Context, t ConsumptionType) (int64, error)`
  - `func (db *DB) GetTypeByName(ctx context.Context, name string) (ConsumptionType, error)`
  - `func (db *DB) ListTypes(ctx context.Context) ([]ConsumptionType, error)`

- [ ] **Step 1: Write the failing test**

`internal/store/types_test.go`:
```go
package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "verbrauch.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCreateAndGetType(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id, err := db.CreateType(ctx, store.ConsumptionType{
		Name:         "oel",
		Unit:         "L",
		PricePerUnit: sql.NullFloat64{Float64: 0.95, Valid: true},
		TankSize:     sql.NullFloat64{Float64: 3000, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateType() error = %v", err)
	}
	if id == 0 {
		t.Fatal("CreateType() returned id = 0")
	}

	got, err := db.GetTypeByName(ctx, "oel")
	if err != nil {
		t.Fatalf("GetTypeByName() error = %v", err)
	}
	if got.Name != "oel" || got.Unit != "L" {
		t.Errorf("got %+v, want Name=oel Unit=L", got)
	}
	if !got.PricePerUnit.Valid || got.PricePerUnit.Float64 != 0.95 {
		t.Errorf("PricePerUnit = %+v, want valid 0.95", got.PricePerUnit)
	}
}

func TestCreateType_Duplicate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.CreateType(ctx, store.ConsumptionType{Name: "strom", Unit: "kWh"}); err != nil {
		t.Fatalf("first CreateType() error = %v", err)
	}
	_, err := db.CreateType(ctx, store.ConsumptionType{Name: "strom", Unit: "kWh"})
	if !errors.Is(err, store.ErrDuplicateType) {
		t.Fatalf("CreateType() duplicate error = %v, want ErrDuplicateType", err)
	}
}

func TestGetTypeByName_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetTypeByName(context.Background(), "does-not-exist")
	if !errors.Is(err, store.ErrTypeNotFound) {
		t.Fatalf("GetTypeByName() error = %v, want ErrTypeNotFound", err)
	}
}

func TestListTypes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.CreateType(ctx, store.ConsumptionType{Name: "strom", Unit: "kWh"}); err != nil {
		t.Fatalf("CreateType(strom) error = %v", err)
	}
	if _, err := db.CreateType(ctx, store.ConsumptionType{Name: "oel", Unit: "L"}); err != nil {
		t.Fatalf("CreateType(oel) error = %v", err)
	}

	types, err := db.ListTypes(ctx)
	if err != nil {
		t.Fatalf("ListTypes() error = %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("len(types) = %d, want 2", len(types))
	}
	// ORDER BY name: "oel" sorts before "strom"
	if types[0].Name != "oel" || types[1].Name != "strom" {
		t.Errorf("types = %+v, want [oel, strom]", types)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/... -run 'TestCreateAndGetType|TestCreateType_Duplicate|TestGetTypeByName_NotFound|TestListTypes' -v`
Expected: FAIL — undefined symbols (`store.ConsumptionType`, `db.CreateType`, ...).

- [ ] **Step 3: Implement the consumption types repository**

`internal/store/types.go`:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ConsumptionType is a configurable meter type (e.g. oil, electricity).
type ConsumptionType struct {
	ID           int64
	Name         string
	Unit         string
	PricePerUnit sql.NullFloat64
	TankSize     sql.NullFloat64
}

var ErrTypeNotFound = errors.New("consumption type not found")
var ErrDuplicateType = errors.New("consumption type already exists")

func (db *DB) CreateType(ctx context.Context, t ConsumptionType) (int64, error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO consumption_types (name, unit, price_per_unit, tank_size) VALUES (?, ?, ?, ?)`,
		t.Name, t.Unit, t.PricePerUnit, t.TankSize)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("%w: %s", ErrDuplicateType, t.Name)
		}
		return 0, fmt.Errorf("insert consumption type %q: %w", t.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read inserted id for %q: %w", t.Name, err)
	}
	return id, nil
}

func (db *DB) GetTypeByName(ctx context.Context, name string) (ConsumptionType, error) {
	var t ConsumptionType
	err := db.QueryRowContext(ctx,
		`SELECT id, name, unit, price_per_unit, tank_size FROM consumption_types WHERE name = ?`, name).
		Scan(&t.ID, &t.Name, &t.Unit, &t.PricePerUnit, &t.TankSize)
	if errors.Is(err, sql.ErrNoRows) {
		return ConsumptionType{}, fmt.Errorf("%w: %s", ErrTypeNotFound, name)
	}
	if err != nil {
		return ConsumptionType{}, fmt.Errorf("get consumption type %q: %w", name, err)
	}
	return t, nil
}

func (db *DB) ListTypes(ctx context.Context) ([]ConsumptionType, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, unit, price_per_unit, tank_size FROM consumption_types ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list consumption types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var types []ConsumptionType
	for rows.Next() {
		var t ConsumptionType
		if err := rows.Scan(&t.ID, &t.Name, &t.Unit, &t.PricePerUnit, &t.TankSize); err != nil {
			return nil, fmt.Errorf("scan consumption type: %w", err)
		}
		types = append(types, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate consumption types: %w", err)
	}
	return types, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/... -run 'TestCreateAndGetType|TestCreateType_Duplicate|TestGetTypeByName_NotFound|TestListTypes' -v`
Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/store/types.go internal/store/types_test.go
git commit -m "feat: add consumption type repository"
```

---

## Task 4: Store — Readings

**Files:**
- Create: `internal/store/readings.go`
- Test: `internal/store/readings_test.go`

**Interfaces:**
- Consumes: `store.Open`, `store.ConsumptionType`, `db.CreateType` (Tasks 2-3)
- Produces:
  - `type Reading struct { ID int64; TypeID int64; ReadingDate string; CounterValue float64; HeatingMode sql.NullString; CreatedAt string }`
  - `var ErrDuplicateReading error`
  - `func (db *DB) InsertReading(ctx context.Context, r Reading) (int64, error)`
  - `func (db *DB) ListReadings(ctx context.Context, typeID int64) ([]Reading, error)` (ascending by `reading_date`)

- [ ] **Step 1: Write the failing test**

`internal/store/readings_test.go`:
```go
package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func mustCreateType(t *testing.T, db *store.DB, name, unit string) store.ConsumptionType {
	t.Helper()
	ctx := context.Background()
	if _, err := db.CreateType(ctx, store.ConsumptionType{Name: name, Unit: unit}); err != nil {
		t.Fatalf("CreateType(%q) error = %v", name, err)
	}
	typ, err := db.GetTypeByName(ctx, name)
	if err != nil {
		t.Fatalf("GetTypeByName(%q) error = %v", name, err)
	}
	return typ
}

func TestInsertAndListReadings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	oel := mustCreateType(t, db, "oel", "L")

	readings := []store.Reading{
		{TypeID: oel.ID, ReadingDate: "2026-01-03", CounterValue: 106, CreatedAt: "2026-01-03T08:00:00Z"},
		{TypeID: oel.ID, ReadingDate: "2026-01-01", CounterValue: 100,
			HeatingMode: sql.NullString{String: "water_and_heat", Valid: true}, CreatedAt: "2026-01-01T08:00:00Z"},
	}
	for _, r := range readings {
		if _, err := db.InsertReading(ctx, r); err != nil {
			t.Fatalf("InsertReading(%+v) error = %v", r, err)
		}
	}

	got, err := db.ListReadings(ctx, oel.ID)
	if err != nil {
		t.Fatalf("ListReadings() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ReadingDate != "2026-01-01" || got[1].ReadingDate != "2026-01-03" {
		t.Errorf("readings not ordered ascending by date: %+v", got)
	}
	if !got[0].HeatingMode.Valid || got[0].HeatingMode.String != "water_and_heat" {
		t.Errorf("HeatingMode = %+v, want valid water_and_heat", got[0].HeatingMode)
	}
}

func TestInsertReading_DuplicateDateForType(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	oel := mustCreateType(t, db, "oel", "L")

	r := store.Reading{TypeID: oel.ID, ReadingDate: "2026-01-01", CounterValue: 100, CreatedAt: "2026-01-01T08:00:00Z"}
	if _, err := db.InsertReading(ctx, r); err != nil {
		t.Fatalf("first InsertReading() error = %v", err)
	}

	r.CounterValue = 105
	_, err := db.InsertReading(ctx, r)
	if !errors.Is(err, store.ErrDuplicateReading) {
		t.Fatalf("second InsertReading() error = %v, want ErrDuplicateReading", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/... -run 'TestInsertAndListReadings|TestInsertReading_DuplicateDateForType' -v`
Expected: FAIL — undefined `store.Reading`.

- [ ] **Step 3: Implement the readings repository**

`internal/store/readings.go`:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Reading is one meter reading (cumulative counter value) for a type on a date.
type Reading struct {
	ID           int64
	TypeID       int64
	ReadingDate  string // YYYY-MM-DD
	CounterValue float64
	HeatingMode  sql.NullString // "off" | "water_only" | "water_and_heat"
	CreatedAt    string         // RFC3339
}

var ErrDuplicateReading = errors.New("reading already exists for this type and date")

func (db *DB) InsertReading(ctx context.Context, r Reading) (int64, error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO readings (type_id, reading_date, counter_value, heating_mode, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		r.TypeID, r.ReadingDate, r.CounterValue, r.HeatingMode, r.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("%w: type_id=%d date=%s", ErrDuplicateReading, r.TypeID, r.ReadingDate)
		}
		return 0, fmt.Errorf("insert reading for type_id=%d date=%s: %w", r.TypeID, r.ReadingDate, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read inserted reading id: %w", err)
	}
	return id, nil
}

func (db *DB) ListReadings(ctx context.Context, typeID int64) ([]Reading, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, type_id, reading_date, counter_value, heating_mode, created_at
		 FROM readings WHERE type_id = ? ORDER BY reading_date ASC`, typeID)
	if err != nil {
		return nil, fmt.Errorf("list readings for type_id=%d: %w", typeID, err)
	}
	defer func() { _ = rows.Close() }()

	var readings []Reading
	for rows.Next() {
		var r Reading
		if err := rows.Scan(&r.ID, &r.TypeID, &r.ReadingDate, &r.CounterValue, &r.HeatingMode, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan reading: %w", err)
		}
		readings = append(readings, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate readings: %w", err)
	}
	return readings, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/... -run 'TestInsertAndListReadings|TestInsertReading_DuplicateDateForType' -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/store/readings.go internal/store/readings_test.go
git commit -m "feat: add readings repository with per-day uniqueness"
```

---

## Task 5: Store — Refills

**Files:**
- Create: `internal/store/refills.go`
- Test: `internal/store/refills_test.go`

**Interfaces:**
- Consumes: `store.Open`, `store.ConsumptionType`, `db.CreateType` (Tasks 2-3)
- Produces:
  - `type Refill struct { ID int64; TypeID int64; RefillDate string; Amount float64; CreatedAt string }`
  - `func (db *DB) InsertRefill(ctx context.Context, r Refill) (int64, error)`
  - `func (db *DB) ListRefills(ctx context.Context, typeID int64) ([]Refill, error)` (ascending by `refill_date`)

- [ ] **Step 1: Write the failing test**

`internal/store/refills_test.go`:
```go
package store_test

import (
	"context"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func TestInsertAndListRefills(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	oel := mustCreateType(t, db, "oel", "L")

	refills := []store.Refill{
		{TypeID: oel.ID, RefillDate: "2026-03-01", Amount: 2000, CreatedAt: "2026-03-01T09:00:00Z"},
		{TypeID: oel.ID, RefillDate: "2026-01-01", Amount: 3000, CreatedAt: "2026-01-01T09:00:00Z"},
	}
	for _, r := range refills {
		if _, err := db.InsertRefill(ctx, r); err != nil {
			t.Fatalf("InsertRefill(%+v) error = %v", r, err)
		}
	}

	got, err := db.ListRefills(ctx, oel.ID)
	if err != nil {
		t.Fatalf("ListRefills() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].RefillDate != "2026-01-01" || got[1].RefillDate != "2026-03-01" {
		t.Errorf("refills not ordered ascending by date: %+v", got)
	}
	if got[0].Amount != 3000 {
		t.Errorf("got[0].Amount = %v, want 3000", got[0].Amount)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/... -run TestInsertAndListRefills -v`
Expected: FAIL — undefined `store.Refill`.

- [ ] **Step 3: Implement the refills repository**

`internal/store/refills.go`:
```go
package store

import (
	"context"
	"fmt"
)

// Refill records a physical top-up of the tank (e.g. an oil delivery).
type Refill struct {
	ID         int64
	TypeID     int64
	RefillDate string // YYYY-MM-DD
	Amount     float64
	CreatedAt  string // RFC3339
}

func (db *DB) InsertRefill(ctx context.Context, r Refill) (int64, error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO refills (type_id, refill_date, amount, created_at) VALUES (?, ?, ?, ?)`,
		r.TypeID, r.RefillDate, r.Amount, r.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert refill for type_id=%d date=%s: %w", r.TypeID, r.RefillDate, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read inserted refill id: %w", err)
	}
	return id, nil
}

func (db *DB) ListRefills(ctx context.Context, typeID int64) ([]Refill, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, type_id, refill_date, amount, created_at
		 FROM refills WHERE type_id = ? ORDER BY refill_date ASC`, typeID)
	if err != nil {
		return nil, fmt.Errorf("list refills for type_id=%d: %w", typeID, err)
	}
	defer func() { _ = rows.Close() }()

	var refills []Refill
	for rows.Next() {
		var r Refill
		if err := rows.Scan(&r.ID, &r.TypeID, &r.RefillDate, &r.Amount, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan refill: %w", err)
		}
		refills = append(refills, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate refills: %w", err)
	}
	return refills, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store/... -run TestInsertAndListRefills -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/refills.go internal/store/refills_test.go
git commit -m "feat: add refills repository"
```

---

## Task 6: Report — Diff Calculation (BuildPoints)

**Files:**
- Create: `internal/report/points.go`
- Test: `internal/report/points_test.go`

**Interfaces:**
- Produces:
  - `type ReadingInput struct { Date time.Time; CounterValue float64; HeatingMode string }`
  - `type DailyPoint struct { Date time.Time; CounterValue float64; HeatingMode string; Diff float64; DiffDays int; Skipped bool }`
  - `func BuildPoints(readings []ReadingInput) (points []DailyPoint, warnings []string)` — `readings` must already be sorted ascending by `Date` (this is how `store.ListReadings` returns them).

- [ ] **Step 1: Write the failing test**

`internal/report/points_test.go`:
```go
package report_test

import (
	"testing"
	"time"

	"github.com/maltekiefer/verbrauch/internal/report"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestBuildPoints(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2026-01-01"), CounterValue: 100},
		{Date: date("2026-01-03"), CounterValue: 106},
		{Date: date("2026-01-04"), CounterValue: 104}, // counter went down: skipped
		{Date: date("2026-01-06"), CounterValue: 112},
	}

	points, warnings := report.BuildPoints(readings)

	if len(points) != 4 {
		t.Fatalf("len(points) = %d, want 4", len(points))
	}

	if points[0].DiffDays != 0 || points[0].Skipped {
		t.Errorf("points[0] (first reading) = %+v, want DiffDays=0 Skipped=false", points[0])
	}

	if points[1].Diff != 6 || points[1].DiffDays != 2 || points[1].Skipped {
		t.Errorf("points[1] = %+v, want Diff=6 DiffDays=2 Skipped=false", points[1])
	}

	if !points[2].Skipped {
		t.Errorf("points[2] = %+v, want Skipped=true (negative diff)", points[2])
	}

	if points[3].Diff != 8 || points[3].DiffDays != 2 || points[3].Skipped {
		t.Errorf("points[3] = %+v, want Diff=8 DiffDays=2 Skipped=false", points[3])
	}

	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1: %v", len(warnings), warnings)
	}
	want := "negativer Verbrauch am 2026-01-04 übersprungen (Zählerstand gesunken von 106.00 auf 104.00)"
	if warnings[0] != want {
		t.Errorf("warnings[0] = %q, want %q", warnings[0], want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/report/... -run TestBuildPoints -v`
Expected: FAIL — `package report: no Go files`.

- [ ] **Step 3: Implement `BuildPoints`**

`internal/report/points.go`:
```go
package report

import (
	"fmt"
	"time"
)

// ReadingInput is a store-independent view of one meter reading, used as
// input to BuildPoints. Callers must pass readings sorted ascending by Date.
type ReadingInput struct {
	Date         time.Time
	CounterValue float64
	HeatingMode  string // "" if not set
}

// DailyPoint is one reading annotated with the consumption since the
// previous reading. Diff/DiffDays are zero for the first point of a series
// and for any point whose computed diff was negative (Skipped = true).
type DailyPoint struct {
	Date         time.Time
	CounterValue float64
	HeatingMode  string
	Diff         float64
	DiffDays     int
	Skipped      bool
}

// BuildPoints turns a sorted series of readings into DailyPoints, computing
// the consumption between each reading and its immediate predecessor. A
// negative diff (counter went down — meter swap or typo) is reported as a
// warning and excluded from Diff/DiffDays on that point.
func BuildPoints(readings []ReadingInput) ([]DailyPoint, []string) {
	var points []DailyPoint
	var warnings []string

	for i, r := range readings {
		p := DailyPoint{
			Date:         r.Date,
			CounterValue: r.CounterValue,
			HeatingMode:  r.HeatingMode,
		}
		if i == 0 {
			points = append(points, p)
			continue
		}

		prev := readings[i-1]
		diff := r.CounterValue - prev.CounterValue
		if diff < 0 {
			warnings = append(warnings, fmt.Sprintf(
				"negativer Verbrauch am %s übersprungen (Zählerstand gesunken von %.2f auf %.2f)",
				r.Date.Format("2006-01-02"), prev.CounterValue, r.CounterValue))
			p.Skipped = true
			points = append(points, p)
			continue
		}

		p.Diff = diff
		p.DiffDays = int(r.Date.Sub(prev.Date).Hours() / 24)
		points = append(points, p)
	}

	return points, warnings
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/report/... -run TestBuildPoints -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/report/points.go internal/report/points_test.go
git commit -m "feat: add report.BuildPoints diff calculation"
```

---

## Task 7: Report — Period Aggregation (AggregateByPeriod)

**Files:**
- Create: `internal/report/aggregate.go`
- Test: `internal/report/aggregate_test.go`

**Interfaces:**
- Consumes: `report.DailyPoint` (Task 6)
- Produces:
  - `type Bucket struct { Label string; Total float64 }`
  - `func AggregateByPeriod(points []DailyPoint, period string, now time.Time) ([]Bucket, error)` — `period` ∈ `{"7d","30d","6m","12m"}`; unknown values return an error.

- [ ] **Step 1: Write the failing test**

`internal/report/aggregate_test.go`:
```go
package report_test

import (
	"testing"

	"github.com/maltekiefer/verbrauch/internal/report"
)

func TestAggregateByPeriod_Daily(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2026-01-01"), CounterValue: 100},
		{Date: date("2026-01-03"), CounterValue: 106}, // +6 over 2 days -> 3.0/day on Jan 2 and Jan 3
		{Date: date("2026-01-04"), CounterValue: 104}, // negative, skipped
		{Date: date("2026-01-06"), CounterValue: 112}, // +8 over 2 days -> 4.0/day on Jan 5 and Jan 6
	}
	points, _ := report.BuildPoints(readings)

	buckets, err := report.AggregateByPeriod(points, "7d", date("2026-01-07"))
	if err != nil {
		t.Fatalf("AggregateByPeriod() error = %v", err)
	}

	want := []report.Bucket{
		{Label: "2026-01-02", Total: 3},
		{Label: "2026-01-03", Total: 3},
		{Label: "2026-01-05", Total: 4},
		{Label: "2026-01-06", Total: 4},
	}
	if len(buckets) != len(want) {
		t.Fatalf("buckets = %+v, want %+v", buckets, want)
	}
	for i := range want {
		if buckets[i] != want[i] {
			t.Errorf("buckets[%d] = %+v, want %+v", i, buckets[i], want[i])
		}
	}
}

func TestAggregateByPeriod_UnknownPeriod(t *testing.T) {
	_, err := report.AggregateByPeriod(nil, "3w", date("2026-01-01"))
	if err == nil {
		t.Fatal("AggregateByPeriod() with unknown period: want error, got nil")
	}
}

func TestAggregateByPeriod_MonthlyBucketing(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2025-10-01"), CounterValue: 0},
		{Date: date("2025-11-01"), CounterValue: 31}, // 1/day over 31 days, all in October/November
	}
	points, _ := report.BuildPoints(readings)

	buckets, err := report.AggregateByPeriod(points, "12m", date("2025-11-02"))
	if err != nil {
		t.Fatalf("AggregateByPeriod() error = %v", err)
	}

	totals := map[string]float64{}
	for _, b := range buckets {
		totals[b.Label] += b.Total
	}
	if _, ok := totals["2025-10"]; !ok {
		t.Errorf("expected a 2025-10 bucket, got %+v", buckets)
	}
	if _, ok := totals["2025-11"]; !ok {
		t.Errorf("expected a 2025-11 bucket, got %+v", buckets)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/report/... -run TestAggregateByPeriod -v`
Expected: FAIL — undefined `report.AggregateByPeriod`.

- [ ] **Step 3: Implement `AggregateByPeriod`**

`internal/report/aggregate.go`:
```go
package report

import (
	"fmt"
	"sort"
	"time"
)

// Bucket is one aggregated slot in a report (a day, a week, or a month).
type Bucket struct {
	Label string
	Total float64
}

type granularity int

const (
	granDay granularity = iota
	granWeek
	granMonth
)

func periodBounds(period string, now time.Time) (time.Time, granularity, error) {
	switch period {
	case "7d":
		return now.AddDate(0, 0, -7), granDay, nil
	case "30d":
		return now.AddDate(0, 0, -30), granDay, nil
	case "6m":
		return now.AddDate(0, -6, 0), granWeek, nil
	case "12m":
		return now.AddDate(-1, 0, 0), granMonth, nil
	default:
		return time.Time{}, 0, fmt.Errorf("unbekannte periode %q, erlaubt: 7d, 30d, 6m, 12m", period)
	}
}

func bucketKey(t time.Time, g granularity) string {
	switch g {
	case granWeek:
		year, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case granMonth:
		return t.Format("2006-01")
	default:
		return t.Format("2006-01-02")
	}
}

// AggregateByPeriod buckets the consumption in points into the given period's
// granularity (7d/30d: daily, 6m: weekly, 12m: monthly), restricted to points
// on or after now minus the period. Each point's Diff is spread evenly across
// the DiffDays days it covers before bucketing, so a reading taken every few
// days still produces a smooth daily/weekly/monthly series.
func AggregateByPeriod(points []DailyPoint, period string, now time.Time) ([]Bucket, error) {
	cutoff, gran, err := periodBounds(period, now)
	if err != nil {
		return nil, err
	}

	totals := map[string]float64{}
	seen := map[string]bool{}
	var order []string

	for _, p := range points {
		if p.Skipped || p.DiffDays == 0 {
			continue
		}
		perDay := p.Diff / float64(p.DiffDays)
		for d := 0; d < p.DiffDays; d++ {
			day := p.Date.AddDate(0, 0, -d)
			if day.Before(cutoff) {
				continue
			}
			key := bucketKey(day, gran)
			if !seen[key] {
				seen[key] = true
				order = append(order, key)
			}
			totals[key] += perDay
		}
	}

	sort.Strings(order)
	buckets := make([]Bucket, 0, len(order))
	for _, key := range order {
		buckets = append(buckets, Bucket{Label: key, Total: totals[key]})
	}
	return buckets, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/report/... -run TestAggregateByPeriod -v`
Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/report/aggregate.go internal/report/aggregate_test.go
git commit -m "feat: add report.AggregateByPeriod bucketing"
```

---

## Task 8: Report — ASCII Chart Rendering

**Files:**
- Create: `internal/report/chart.go`
- Test: `internal/report/chart_test.go`

**Interfaces:**
- Consumes: `report.Bucket` (Task 7)
- Produces: `func RenderChart(buckets []Bucket) string`

- [ ] **Step 1: Write the failing test**

`internal/report/chart_test.go`:
```go
package report_test

import (
	"strings"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/report"
)

func TestRenderChart(t *testing.T) {
	buckets := []report.Bucket{
		{Label: "2026-01-01", Total: 10},
		{Label: "2026-01-02", Total: 20},
	}

	got := report.RenderChart(buckets)

	if !strings.Contains(got, "2026-01-01") || !strings.Contains(got, "2026-01-02") {
		t.Fatalf("chart missing labels: %q", got)
	}
	if !strings.Contains(got, strings.Repeat("█", 20)) {
		t.Errorf("chart missing full-width (40-wide max) bar for the larger value: %q", got)
	}
	if !strings.Contains(got, strings.Repeat("█", 40)) {
		t.Errorf("chart missing full 40-char bar for the max value: %q", got)
	}
	if !strings.Contains(got, "10.0") || !strings.Contains(got, "20.0") {
		t.Errorf("chart missing numeric totals: %q", got)
	}
}

func TestRenderChart_Empty(t *testing.T) {
	got := report.RenderChart(nil)
	if !strings.Contains(got, "keine Daten") {
		t.Errorf("RenderChart(nil) = %q, want a no-data message", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/report/... -run TestRenderChart -v`
Expected: FAIL — undefined `report.RenderChart`.

- [ ] **Step 3: Implement `RenderChart`**

`internal/report/chart.go`:
```go
package report

import (
	"fmt"
	"strings"
)

const chartWidth = 40

// RenderChart draws a horizontal ASCII/Unicode bar chart, one line per
// bucket, scaled so the largest total fills chartWidth characters.
func RenderChart(buckets []Bucket) string {
	if len(buckets) == 0 {
		return "(keine Daten für Diagramm)\n"
	}

	max := 0.0
	for _, b := range buckets {
		if b.Total > max {
			max = b.Total
		}
	}

	var sb strings.Builder
	for _, b := range buckets {
		barLen := 0
		if max > 0 {
			barLen = int(b.Total / max * chartWidth)
		}
		bar := strings.Repeat("█", barLen)
		fmt.Fprintf(&sb, "%-10s %s %.1f\n", b.Label, bar, b.Total)
	}
	return sb.String()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/report/... -run TestRenderChart -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/report/chart.go internal/report/chart_test.go
git commit -m "feat: add ASCII chart rendering"
```

---

## Task 9: Forecast — Trend Calculation

**Files:**
- Create: `internal/forecast/trend.go`
- Test: `internal/forecast/trend_test.go`

**Interfaces:**
- Consumes: `report.DailyPoint` (Task 6)
- Produces:
  - `var ErrInsufficientData error`
  - `type Trend struct { AvgPerDay float64 }`
  - `func ComputeTrend(points []report.DailyPoint, windowDays int, now time.Time) (Trend, error)`

- [ ] **Step 1: Write the failing test**

`internal/forecast/trend_test.go`:
```go
package forecast_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/maltekiefer/verbrauch/internal/forecast"
	"github.com/maltekiefer/verbrauch/internal/report"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestComputeTrend(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2026-01-01"), CounterValue: 100},
		{Date: date("2026-01-03"), CounterValue: 106}, // Diff=6, DiffDays=2
		{Date: date("2026-01-04"), CounterValue: 104}, // negative, skipped
		{Date: date("2026-01-06"), CounterValue: 112}, // Diff=8, DiffDays=2
	}
	points, _ := report.BuildPoints(readings)

	trend, err := forecast.ComputeTrend(points, 30, date("2026-01-10"))
	if err != nil {
		t.Fatalf("ComputeTrend() error = %v", err)
	}

	want := 3.5 // (6+8) / (2+2)
	if math.Abs(trend.AvgPerDay-want) > 0.001 {
		t.Errorf("AvgPerDay = %v, want %v", trend.AvgPerDay, want)
	}
}

func TestComputeTrend_InsufficientData(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2026-01-01"), CounterValue: 100},
	}
	points, _ := report.BuildPoints(readings)

	_, err := forecast.ComputeTrend(points, 30, date("2026-01-10"))
	if !errors.Is(err, forecast.ErrInsufficientData) {
		t.Fatalf("ComputeTrend() error = %v, want ErrInsufficientData", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/forecast/... -run TestComputeTrend -v`
Expected: FAIL — `package forecast: no Go files`.

- [ ] **Step 3: Implement `ComputeTrend`**

`internal/forecast/trend.go`:
```go
package forecast

import (
	"errors"
	"time"

	"github.com/maltekiefer/verbrauch/internal/report"
)

var ErrInsufficientData = errors.New("zu wenig Daten für Prognose")

// Trend is the average consumption per day over a lookback window.
type Trend struct {
	AvgPerDay float64
}

// ComputeTrend averages consumption per day over the windowDays before now,
// using only points with a valid (non-skipped, non-zero) diff. It returns
// ErrInsufficientData if no such point falls in the window.
func ComputeTrend(points []report.DailyPoint, windowDays int, now time.Time) (Trend, error) {
	cutoff := now.AddDate(0, 0, -windowDays)

	var totalDiff float64
	var totalDays int

	for _, p := range points {
		if p.Skipped || p.DiffDays == 0 {
			continue
		}
		if p.Date.Before(cutoff) {
			continue
		}
		totalDiff += p.Diff
		totalDays += p.DiffDays
	}

	if totalDays == 0 {
		return Trend{}, ErrInsufficientData
	}
	return Trend{AvgPerDay: totalDiff / float64(totalDays)}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/forecast/... -run TestComputeTrend -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/forecast/trend.go internal/forecast/trend_test.go
git commit -m "feat: add forecast.ComputeTrend"
```

---

## Task 10: Forecast — Tank Range and Projection

**Files:**
- Create: `internal/forecast/project.go`
- Test: `internal/forecast/project_test.go`

**Interfaces:**
- Consumes: `report.DailyPoint`, `forecast.Trend` (Tasks 6, 9)
- Produces:
  - `type RefillInput struct { Date time.Time; Amount float64 }`
  - `func TankRemaining(refills []RefillInput, points []report.DailyPoint) (remaining float64, ok bool)`
  - `type Projection struct { HorizonDays int; Projected float64; Cost *float64; DaysUntilEmpty *float64 }`
  - `func Project(trend Trend, horizonDays int, pricePerUnit *float64, tankRemaining *float64) Projection`

- [ ] **Step 1: Write the failing test**

`internal/forecast/project_test.go`:
```go
package forecast_test

import (
	"math"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/forecast"
	"github.com/maltekiefer/verbrauch/internal/report"
)

func TestTankRemaining(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2026-01-01"), CounterValue: 100},
		{Date: date("2026-01-03"), CounterValue: 106}, // Diff=6, after refill
		{Date: date("2026-01-04"), CounterValue: 104}, // negative, skipped
		{Date: date("2026-01-06"), CounterValue: 112}, // Diff=8, after refill
	}
	points, _ := report.BuildPoints(readings)

	refills := []forecast.RefillInput{{Date: date("2026-01-01"), Amount: 1000}}

	remaining, ok := forecast.TankRemaining(refills, points)
	if !ok {
		t.Fatal("TankRemaining() ok = false, want true")
	}
	want := 1000.0 - 14.0 // 1000 - (6+8) consumed strictly after the refill date
	if math.Abs(remaining-want) > 0.001 {
		t.Errorf("remaining = %v, want %v", remaining, want)
	}
}

func TestTankRemaining_NoRefills(t *testing.T) {
	_, ok := forecast.TankRemaining(nil, nil)
	if ok {
		t.Fatal("TankRemaining() with no refills: ok = true, want false")
	}
}

func TestProject(t *testing.T) {
	trend := forecast.Trend{AvgPerDay: 3.5}
	price := 0.9
	remaining := 986.0

	proj := forecast.Project(trend, 7, &price, &remaining)

	if math.Abs(proj.Projected-24.5) > 0.001 {
		t.Errorf("Projected = %v, want 24.5", proj.Projected)
	}
	if proj.Cost == nil || math.Abs(*proj.Cost-22.05) > 0.001 {
		t.Errorf("Cost = %v, want 22.05", proj.Cost)
	}
	wantDays := 986.0 / 3.5
	if proj.DaysUntilEmpty == nil || math.Abs(*proj.DaysUntilEmpty-wantDays) > 0.001 {
		t.Errorf("DaysUntilEmpty = %v, want %v", proj.DaysUntilEmpty, wantDays)
	}
}

func TestProject_NoPriceNoTank(t *testing.T) {
	trend := forecast.Trend{AvgPerDay: 2.0}

	proj := forecast.Project(trend, 30, nil, nil)

	if proj.Cost != nil {
		t.Errorf("Cost = %v, want nil", proj.Cost)
	}
	if proj.DaysUntilEmpty != nil {
		t.Errorf("DaysUntilEmpty = %v, want nil", proj.DaysUntilEmpty)
	}
	if math.Abs(proj.Projected-60.0) > 0.001 {
		t.Errorf("Projected = %v, want 60.0", proj.Projected)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/forecast/... -run 'TestTankRemaining|TestProject' -v`
Expected: FAIL — undefined `forecast.RefillInput`, `forecast.Project`.

- [ ] **Step 3: Implement `TankRemaining` and `Project`**

`internal/forecast/project.go`:
```go
package forecast

import (
	"time"

	"github.com/maltekiefer/verbrauch/internal/report"
)

// RefillInput is a store-independent view of one tank refill event.
type RefillInput struct {
	Date   time.Time
	Amount float64
}

// TankRemaining computes the estimated amount left in the tank, based on the
// most recent refill minus every valid diff on or after that refill's date.
// It returns ok=false if there are no refills to anchor the calculation to.
func TankRemaining(refills []RefillInput, points []report.DailyPoint) (float64, bool) {
	if len(refills) == 0 {
		return 0, false
	}

	last := refills[len(refills)-1]
	var consumed float64
	for _, p := range points {
		if p.Skipped || p.DiffDays == 0 {
			continue
		}
		if p.Date.After(last.Date) {
			consumed += p.Diff
		}
	}
	return last.Amount - consumed, true
}

// Projection is the forecast result for a given horizon.
type Projection struct {
	HorizonDays    int
	Projected      float64
	Cost           *float64
	DaysUntilEmpty *float64
}

// Project extrapolates trend.AvgPerDay over horizonDays, optionally adding a
// cost projection (if pricePerUnit is given) and a days-until-empty estimate
// (if tankRemaining is given and the trend is positive).
func Project(trend Trend, horizonDays int, pricePerUnit *float64, tankRemaining *float64) Projection {
	proj := Projection{
		HorizonDays: horizonDays,
		Projected:   trend.AvgPerDay * float64(horizonDays),
	}

	if pricePerUnit != nil {
		cost := proj.Projected * *pricePerUnit
		proj.Cost = &cost
	}

	if tankRemaining != nil && trend.AvgPerDay > 0 {
		days := *tankRemaining / trend.AvgPerDay
		proj.DaysUntilEmpty = &days
	}

	return proj
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/forecast/... -run 'TestTankRemaining|TestProject' -v`
Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/forecast/project.go internal/forecast/project_test.go
git commit -m "feat: add tank range and cost/consumption projection"
```

---

## Task 11: CLI — Root Command and Type Management

**Files:**
- Create: `internal/cli/root.go`
- Create: `internal/cli/type.go`
- Test: `internal/cli/type_test.go`

**Interfaces:**
- Consumes: `store.Open`, `store.DB`, `store.ConsumptionType` (Tasks 2-3)
- Produces:
  - `type UsageError struct{ msg string }` implementing `error`
  - `func NewRootCmd() *cobra.Command`
  - (internal) `type App struct { dbPath string; json bool; db *store.DB }`
  - (internal) `func resolveDBPath(flagValue string) (string, error)`

- [ ] **Step 1: Write the failing test**

`internal/cli/type_test.go`:
```go
package cli_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/cli"
)

func TestTypeAddAndList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	addCmd := cli.NewRootCmd()
	addCmd.SetOut(io.Discard)
	addCmd.SetErr(io.Discard)
	addCmd.SetArgs([]string{"--db", dbPath, "type", "add", "oel", "--unit", "L", "--price", "0.95", "--tank-size", "3000"})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("type add: %v", err)
	}

	var out bytes.Buffer
	listCmd := cli.NewRootCmd()
	listCmd.SetOut(&out)
	listCmd.SetErr(io.Discard)
	listCmd.SetArgs([]string{"--db", dbPath, "type", "list"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("type list: %v", err)
	}

	got := out.String()
	for _, want := range []string{"oel", "L", "0.95", "3000"} {
		if !strings.Contains(got, want) {
			t.Errorf("type list output %q missing %q", got, want)
		}
	}
}

func TestTypeAdd_DuplicateIsUsageStyleError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	first := cli.NewRootCmd()
	first.SetOut(io.Discard)
	first.SetErr(io.Discard)
	first.SetArgs([]string{"--db", dbPath, "type", "add", "oel", "--unit", "L"})
	if err := first.Execute(); err != nil {
		t.Fatalf("first type add: %v", err)
	}

	second := cli.NewRootCmd()
	second.SetOut(io.Discard)
	second.SetErr(io.Discard)
	second.SetArgs([]string{"--db", dbPath, "type", "add", "oel", "--unit", "L"})
	if err := second.Execute(); err == nil {
		t.Fatal("second type add: want error for duplicate name, got nil")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli/... -run TestTypeAdd -v`
Expected: FAIL — `package cli: no Go files`.

- [ ] **Step 3: Implement the root command**

`internal/cli/root.go`:
```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

// UsageError marks an error caused by invalid user input (a bad flag value,
// an unknown type name, ...), as opposed to an internal/runtime failure.
// main.go maps this to exit code 2.
type UsageError struct {
	msg string
}

func (e *UsageError) Error() string { return e.msg }

func newUsageError(format string, args ...any) error {
	return &UsageError{msg: fmt.Sprintf(format, args...)}
}

// App holds the state shared across a single command invocation.
type App struct {
	dbPath string
	json   bool
	db     *store.DB
}

// NewRootCmd builds the verbrauch command tree.
func NewRootCmd() *cobra.Command {
	app := &App{}

	root := &cobra.Command{
		Use:   "verbrauch",
		Short: "Erfasst und wertet Verbrauchszählerstände aus (Öl, Strom, ...)",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(app.dbPath)
			if err != nil {
				return err
			}
			db, err := store.Open(cmd.Context(), path)
			if err != nil {
				return err
			}
			app.db = db
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if app.db == nil {
				return nil
			}
			return app.db.Close()
		},
	}
	root.SilenceUsage = true
	root.SilenceErrors = true

	root.PersistentFlags().StringVar(&app.dbPath, "db", "",
		"Pfad zur SQLite-Datenbank (Default: plattformspezifischer Konfigordner)")
	root.PersistentFlags().BoolVar(&app.json, "json", false, "Ausgabe als JSON")

	root.AddCommand(newTypeCmd(app))

	return root
}

func resolveDBPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if envPath := os.Getenv("VERBRAUCH_DB"); envPath != "" {
		return envPath, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("konfigverzeichnis ermitteln: %w", err)
	}
	return filepath.Join(configDir, "verbrauch", "verbrauch.db"), nil
}
```

- [ ] **Step 4: Implement the `type` command**

`internal/cli/type.go`:
```go
package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func newTypeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "type",
		Short: "Verbrauchsarten verwalten",
	}
	cmd.AddCommand(newTypeAddCmd(app))
	cmd.AddCommand(newTypeListCmd(app))
	return cmd
}

func newTypeAddCmd(app *App) *cobra.Command {
	var unit string
	var price, tankSize float64

	cmd := &cobra.Command{
		Use:     "add <name>",
		Short:   "Neue Verbrauchsart anlegen",
		Args:    cobra.ExactArgs(1),
		Example: "  verbrauch type add oel --unit L --price 0.95 --tank-size 3000",
		RunE: func(cmd *cobra.Command, args []string) error {
			t := store.ConsumptionType{Name: args[0], Unit: unit}
			if cmd.Flags().Changed("price") {
				t.PricePerUnit = sql.NullFloat64{Float64: price, Valid: true}
			}
			if cmd.Flags().Changed("tank-size") {
				t.TankSize = sql.NullFloat64{Float64: tankSize, Valid: true}
			}

			id, err := app.db.CreateType(cmd.Context(), t)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verbrauchsart %q angelegt (id=%d)\n", t.Name, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "Einheit, z.B. L oder kWh")
	_ = cmd.MarkFlagRequired("unit")
	cmd.Flags().Float64Var(&price, "price", 0, "Preis pro Einheit (optional)")
	cmd.Flags().Float64Var(&tankSize, "tank-size", 0, "Tankgröße für Reichweitenberechnung (optional)")
	return cmd
}

type typeDTO struct {
	Name         string   `json:"name"`
	Unit         string   `json:"unit"`
	PricePerUnit *float64 `json:"price_per_unit,omitempty"`
	TankSize     *float64 `json:"tank_size,omitempty"`
}

func toTypeDTO(t store.ConsumptionType) typeDTO {
	dto := typeDTO{Name: t.Name, Unit: t.Unit}
	if t.PricePerUnit.Valid {
		dto.PricePerUnit = &t.PricePerUnit.Float64
	}
	if t.TankSize.Valid {
		dto.TankSize = &t.TankSize.Float64
	}
	return dto
}

func newTypeListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "Alle Verbrauchsarten auflisten",
		Example: "  verbrauch type list",
		RunE: func(cmd *cobra.Command, args []string) error {
			types, err := app.db.ListTypes(cmd.Context())
			if err != nil {
				return err
			}

			if app.json {
				dtos := make([]typeDTO, len(types))
				for i, t := range types {
					dtos[i] = toTypeDTO(t)
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(dtos)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tEINHEIT\tPREIS\tTANKGRÖSSE")
			for _, t := range types {
				price, tank := "-", "-"
				if t.PricePerUnit.Valid {
					price = fmt.Sprintf("%.2f", t.PricePerUnit.Float64)
				}
				if t.TankSize.Valid {
					tank = fmt.Sprintf("%.0f", t.TankSize.Float64)
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, t.Unit, price, tank)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("ausgabe schreiben: %w", err)
			}
			return nil
		},
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/... -run TestTypeAdd -v`
Expected: PASS for both tests.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/root.go internal/cli/type.go internal/cli/type_test.go
git commit -m "feat: add cli root command and type management"
```

---

## Task 12: CLI — Reading and Refill Entry

**Files:**
- Create: `internal/cli/add.go`
- Create: `internal/cli/refill.go`
- Modify: `internal/cli/root.go` (register the two new commands)
- Test: `internal/cli/add_test.go`

**Interfaces:**
- Consumes: `App`, `newUsageError`, `store.Reading`, `store.Refill` (Tasks 4, 5, 11)
- Produces:
  - `func newAddCmd(app *App) *cobra.Command` (registered as `verbrauch add`)
  - `func newRefillCmd(app *App) *cobra.Command` (registered as `verbrauch refill`)

- [ ] **Step 1: Write the failing test**

`internal/cli/add_test.go`:
```go
package cli_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/cli"
)

func setupType(t *testing.T, dbPath, name, unit string) {
	t.Helper()
	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "type", "add", name, "--unit", unit})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("setupType(%q): %v", name, err)
	}
}

func TestAddReading(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", "oel", "--value", "45231.5", "--date", "2026-09-19", "--heating", "both"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out.String(), "erfasst") {
		t.Errorf("add output = %q, want confirmation containing 'erfasst'", out.String())
	}
}

func TestAddReading_InvalidHeatingIsUsageError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")

	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", "oel", "--value", "1", "--heating", "invalid"})
	err := cmd.Execute()
	var usageErr *cli.UsageError
	if err == nil {
		t.Fatal("add with invalid --heating: want error, got nil")
	}
	if !errors.As(err, &usageErr) {
		t.Errorf("add with invalid --heating: error = %v, want *cli.UsageError", err)
	}
}

func TestAddReading_UnknownTypeIsUsageError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", "does-not-exist", "--value", "1"})
	err := cmd.Execute()
	var usageErr *cli.UsageError
	if err == nil {
		t.Fatal("add with unknown --type: want error, got nil")
	}
	if !errors.As(err, &usageErr) {
		t.Errorf("add with unknown --type: error = %v, want *cli.UsageError", err)
	}
}

func TestRefill(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "refill", "--type", "oel", "--amount", "2500", "--date", "2026-09-19"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("refill: %v", err)
	}
	if !strings.Contains(out.String(), "erfasst") {
		t.Errorf("refill output = %q, want confirmation containing 'erfasst'", out.String())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli/... -run 'TestAddReading|TestRefill' -v`
Expected: FAIL — undefined command `add`/`refill` (cobra reports `unknown command`).

- [ ] **Step 3: Implement the `add` command**

`internal/cli/add.go`:
```go
package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func newAddCmd(app *App) *cobra.Command {
	var typeName, dateStr, heating string
	var value float64

	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Zählerstand erfassen",
		Example: "  verbrauch add --type oel --value 45231.5 --heating both",
		RunE: func(cmd *cobra.Command, args []string) error {
			date := dateStr
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return newUsageError("ungültiges Datum %q, erwartet YYYY-MM-DD", date)
			}

			var mode sql.NullString
			switch heating {
			case "":
				// kein Wert
			case "off":
				mode = sql.NullString{String: "off", Valid: true}
			case "water":
				mode = sql.NullString{String: "water_only", Valid: true}
			case "both":
				mode = sql.NullString{String: "water_and_heat", Valid: true}
			default:
				return newUsageError("ungültiger --heating Wert %q, erlaubt: off, water, both", heating)
			}

			t, err := app.db.GetTypeByName(cmd.Context(), typeName)
			if err != nil {
				if errors.Is(err, store.ErrTypeNotFound) {
					return newUsageError("Verbrauchsart %q nicht gefunden, siehe 'verbrauch type list'", typeName)
				}
				return err
			}

			id, err := app.db.InsertReading(cmd.Context(), store.Reading{
				TypeID:       t.ID,
				ReadingDate:  date,
				CounterValue: value,
				HeatingMode:  mode,
				CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Zählerstand erfasst (id=%d)\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart (siehe 'verbrauch type list')")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().Float64Var(&value, "value", 0, "Zählerstand")
	_ = cmd.MarkFlagRequired("value")
	cmd.Flags().StringVar(&dateStr, "date", "", "Datum YYYY-MM-DD (Default: heute)")
	cmd.Flags().StringVar(&heating, "heating", "", "Heizmodus: off, water, both (optional)")
	return cmd
}
```

- [ ] **Step 4: Implement the `refill` command and register both commands**

`internal/cli/refill.go`:
```go
package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func newRefillCmd(app *App) *cobra.Command {
	var typeName, dateStr string
	var amount float64

	cmd := &cobra.Command{
		Use:     "refill",
		Short:   "Tankbefüllung erfassen",
		Example: "  verbrauch refill --type oel --amount 2500",
		RunE: func(cmd *cobra.Command, args []string) error {
			date := dateStr
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return newUsageError("ungültiges Datum %q, erwartet YYYY-MM-DD", date)
			}

			t, err := app.db.GetTypeByName(cmd.Context(), typeName)
			if err != nil {
				if errors.Is(err, store.ErrTypeNotFound) {
					return newUsageError("Verbrauchsart %q nicht gefunden, siehe 'verbrauch type list'", typeName)
				}
				return err
			}

			id, err := app.db.InsertRefill(cmd.Context(), store.Refill{
				TypeID:     t.ID,
				RefillDate: date,
				Amount:     amount,
				CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Befüllung erfasst (id=%d)\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().Float64Var(&amount, "amount", 0, "Nachgefüllte Menge")
	_ = cmd.MarkFlagRequired("amount")
	cmd.Flags().StringVar(&dateStr, "date", "", "Datum YYYY-MM-DD (Default: heute)")
	return cmd
}
```

In `internal/cli/root.go`, register the two new commands in `NewRootCmd` right after `root.AddCommand(newTypeCmd(app))`:
```go
	root.AddCommand(newAddCmd(app))
	root.AddCommand(newRefillCmd(app))
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/... -run 'TestAddReading|TestRefill' -v`
Expected: PASS for all four tests.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/add.go internal/cli/refill.go internal/cli/root.go internal/cli/add_test.go
git commit -m "feat: add reading and refill entry commands"
```

---

## Task 13: CLI — Report, Forecast, and Status

**Files:**
- Create: `internal/cli/report.go`
- Create: `internal/cli/forecast.go`
- Create: `internal/cli/status.go`
- Modify: `internal/cli/root.go` (register the three new commands)
- Test: `internal/cli/report_test.go`

**Interfaces:**
- Consumes: `report.BuildPoints`, `report.AggregateByPeriod`, `report.RenderChart`, `forecast.ComputeTrend`, `forecast.TankRemaining`, `forecast.Project` (Tasks 6-10), `App`, `newUsageError` (Task 11)
- Produces:
  - `func newReportCmd(app *App) *cobra.Command` (`verbrauch report`)
  - `func newForecastCmd(app *App) *cobra.Command` (`verbrauch forecast`)
  - `func newStatusCmd(app *App) *cobra.Command` (`verbrauch status`)
  - (internal, shared) `func toReadingInputs(readings []store.Reading) []report.ReadingInput`
  - (internal, shared) `func toRefillInputs(refills []store.Refill) []forecast.RefillInput`

- [ ] **Step 1: Write the failing test**

`internal/cli/report_test.go`:
```go
package cli_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/cli"
)

func addReading(t *testing.T, dbPath, typeName, date string, value float64) {
	t.Helper()
	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", typeName, "--value", floatStr(value), "--date", date})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("addReading(%s, %v): %v", date, value, err)
	}
}

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func TestReportAndForecastAndStatus(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")
	addReading(t, dbPath, "oel", "2026-01-01", 100)
	addReading(t, dbPath, "oel", "2026-01-03", 106)
	addReading(t, dbPath, "oel", "2026-01-06", 112)

	var reportOut bytes.Buffer
	reportCmd := cli.NewRootCmd()
	reportCmd.SetOut(&reportOut)
	reportCmd.SetErr(io.Discard)
	reportCmd.SetArgs([]string{"--db", dbPath, "report", "--type", "oel", "--period", "30d"})
	if err := reportCmd.Execute(); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(reportOut.String(), "ZEITRAUM") {
		t.Errorf("report output = %q, want a table header", reportOut.String())
	}

	var forecastOut bytes.Buffer
	forecastCmd := cli.NewRootCmd()
	forecastCmd.SetOut(&forecastOut)
	forecastCmd.SetErr(io.Discard)
	forecastCmd.SetArgs([]string{"--db", dbPath, "forecast", "--type", "oel", "--horizon", "7d"})
	if err := forecastCmd.Execute(); err != nil {
		t.Fatalf("forecast: %v", err)
	}
	if !strings.Contains(forecastOut.String(), "Prognose") {
		t.Errorf("forecast output = %q, want a Prognose line", forecastOut.String())
	}

	var statusOut bytes.Buffer
	statusCmd := cli.NewRootCmd()
	statusCmd.SetOut(&statusOut)
	statusCmd.SetErr(io.Discard)
	statusCmd.SetArgs([]string{"--db", dbPath, "status", "--type", "oel"})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(statusOut.String(), "oel") {
		t.Errorf("status output = %q, want it to mention the type name", statusOut.String())
	}
}

func TestForecast_InvalidHorizonIsUsageError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")
	addReading(t, dbPath, "oel", "2026-01-01", 100)
	addReading(t, dbPath, "oel", "2026-01-03", 106)

	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "forecast", "--type", "oel", "--horizon", "1y"})
	err := cmd.Execute()
	var usageErr *cli.UsageError
	if err == nil {
		t.Fatal("forecast with invalid --horizon: want error, got nil")
	}
	if !errors.As(err, &usageErr) {
		t.Errorf("forecast with invalid --horizon: error = %v, want *cli.UsageError", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli/... -run 'TestReportAndForecastAndStatus|TestForecast_InvalidHorizon' -v`
Expected: FAIL — undefined command `report`/`forecast`/`status`.

- [ ] **Step 3: Implement shared conversion helpers and the `report` command**

`internal/cli/report.go`:
```go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/report"
	"github.com/maltekiefer/verbrauch/internal/store"
)

func toReadingInputs(readings []store.Reading) []report.ReadingInput {
	inputs := make([]report.ReadingInput, len(readings))
	for i, r := range readings {
		d, _ := time.Parse("2006-01-02", r.ReadingDate)
		mode := ""
		if r.HeatingMode.Valid {
			mode = r.HeatingMode.String
		}
		inputs[i] = report.ReadingInput{Date: d, CounterValue: r.CounterValue, HeatingMode: mode}
	}
	return inputs
}

var validPeriods = map[string]bool{"7d": true, "30d": true, "6m": true, "12m": true}

func newReportCmd(app *App) *cobra.Command {
	var typeName, period string

	cmd := &cobra.Command{
		Use:     "report",
		Short:   "Verbrauchsbericht (Tabelle + Diagramm)",
		Example: "  verbrauch report --type oel --period 30d",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validPeriods[period] {
				return newUsageError("ungültiger --period Wert %q, erlaubt: 7d, 30d, 6m, 12m", period)
			}

			t, err := app.db.GetTypeByName(cmd.Context(), typeName)
			if err != nil {
				if errors.Is(err, store.ErrTypeNotFound) {
					return newUsageError("Verbrauchsart %q nicht gefunden, siehe 'verbrauch type list'", typeName)
				}
				return err
			}

			readings, err := app.db.ListReadings(cmd.Context(), t.ID)
			if err != nil {
				return err
			}
			points, warnings := report.BuildPoints(toReadingInputs(readings))
			for _, w := range warnings {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warnung:", w)
			}

			buckets, err := report.AggregateByPeriod(points, period, time.Now())
			if err != nil {
				return newUsageError("%s", err.Error())
			}

			if app.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(buckets)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintf(w, "ZEITRAUM\tVERBRAUCH (%s)\n", t.Unit)
			for _, b := range buckets {
				_, _ = fmt.Fprintf(w, "%s\t%.1f\n", b.Label, b.Total)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("ausgabe schreiben: %w", err)
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprint(cmd.OutOrStdout(), report.RenderChart(buckets))
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().StringVar(&period, "period", "30d", "Zeitraum: 7d, 30d, 6m, 12m")
	return cmd
}
```

- [ ] **Step 4: Implement the `forecast` command**

`internal/cli/forecast.go`:
```go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/forecast"
	"github.com/maltekiefer/verbrauch/internal/report"
	"github.com/maltekiefer/verbrauch/internal/store"
)

func toRefillInputs(refills []store.Refill) []forecast.RefillInput {
	inputs := make([]forecast.RefillInput, len(refills))
	for i, r := range refills {
		d, _ := time.Parse("2006-01-02", r.RefillDate)
		inputs[i] = forecast.RefillInput{Date: d, Amount: r.Amount}
	}
	return inputs
}

func parseHorizonDays(horizon string) (int, error) {
	switch horizon {
	case "7d":
		return 7, nil
	case "30d":
		return 30, nil
	default:
		return 0, newUsageError("ungültiger --horizon Wert %q, erlaubt: 7d, 30d", horizon)
	}
}

func newForecastCmd(app *App) *cobra.Command {
	var typeName, horizon string

	cmd := &cobra.Command{
		Use:     "forecast",
		Short:   "Verbrauchs-, Kosten- und Reichweitenprognose",
		Example: "  verbrauch forecast --type oel --horizon 30d",
		RunE: func(cmd *cobra.Command, args []string) error {
			horizonDays, err := parseHorizonDays(horizon)
			if err != nil {
				return err
			}

			t, err := app.db.GetTypeByName(cmd.Context(), typeName)
			if err != nil {
				if errors.Is(err, store.ErrTypeNotFound) {
					return newUsageError("Verbrauchsart %q nicht gefunden, siehe 'verbrauch type list'", typeName)
				}
				return err
			}

			readings, err := app.db.ListReadings(cmd.Context(), t.ID)
			if err != nil {
				return err
			}
			points, warnings := report.BuildPoints(toReadingInputs(readings))
			for _, w := range warnings {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warnung:", w)
			}

			trend, err := forecast.ComputeTrend(points, 30, time.Now())
			if err != nil {
				return err
			}

			var pricePtr *float64
			if t.PricePerUnit.Valid {
				pricePtr = &t.PricePerUnit.Float64
			}

			var tankRemainingPtr *float64
			if t.TankSize.Valid {
				refills, err := app.db.ListRefills(cmd.Context(), t.ID)
				if err != nil {
					return err
				}
				if remaining, ok := forecast.TankRemaining(toRefillInputs(refills), points); ok {
					tankRemainingPtr = &remaining
				}
			}

			projection := forecast.Project(trend, horizonDays, pricePtr, tankRemainingPtr)

			if app.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(projection)
			}

			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Durchschnitt: %.2f %s/Tag\n", trend.AvgPerDay, t.Unit)
			_, _ = fmt.Fprintf(out, "Prognose (%d Tage): %.1f %s\n", projection.HorizonDays, projection.Projected, t.Unit)
			if projection.Cost != nil {
				_, _ = fmt.Fprintf(out, "Kostenprognose: %.2f\n", *projection.Cost)
			}
			if projection.DaysUntilEmpty != nil {
				_, _ = fmt.Fprintf(out, "Tage bis Tank leer: %.1f\n", *projection.DaysUntilEmpty)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().StringVar(&horizon, "horizon", "30d", "Prognosehorizont: 7d oder 30d")
	return cmd
}
```

- [ ] **Step 5: Implement the `status` command and register all three**

`internal/cli/status.go`:
```go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/forecast"
	"github.com/maltekiefer/verbrauch/internal/report"
	"github.com/maltekiefer/verbrauch/internal/store"
)

type statusOutput struct {
	Type           string   `json:"type"`
	LastPerDay     *float64 `json:"last_per_day,omitempty"`
	TankRemaining  *float64 `json:"tank_remaining,omitempty"`
	DaysUntilEmpty *float64 `json:"days_until_empty,omitempty"`
}

func newStatusCmd(app *App) *cobra.Command {
	var typeName string

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Kurzübersicht: Füllstand, Reichweite, letzter Verbrauch",
		Example: "  verbrauch status --type oel",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := app.db.GetTypeByName(cmd.Context(), typeName)
			if err != nil {
				if errors.Is(err, store.ErrTypeNotFound) {
					return newUsageError("Verbrauchsart %q nicht gefunden, siehe 'verbrauch type list'", typeName)
				}
				return err
			}

			readings, err := app.db.ListReadings(cmd.Context(), t.ID)
			if err != nil {
				return err
			}
			points, warnings := report.BuildPoints(toReadingInputs(readings))
			for _, w := range warnings {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warnung:", w)
			}

			out := statusOutput{Type: t.Name}
			if len(points) > 0 {
				last := points[len(points)-1]
				if !last.Skipped && last.DiffDays > 0 {
					perDay := last.Diff / float64(last.DiffDays)
					out.LastPerDay = &perDay
				}
			}

			if t.TankSize.Valid {
				refills, err := app.db.ListRefills(cmd.Context(), t.ID)
				if err != nil {
					return err
				}
				if remaining, ok := forecast.TankRemaining(toRefillInputs(refills), points); ok {
					out.TankRemaining = &remaining
					if trend, err := forecast.ComputeTrend(points, 30, time.Now()); err == nil && trend.AvgPerDay > 0 {
						days := remaining / trend.AvgPerDay
						out.DaysUntilEmpty = &days
					}
				}
			}

			if app.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
			}

			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "Verbrauchsart: %s\n", out.Type)
			if out.LastPerDay != nil {
				_, _ = fmt.Fprintf(w, "Letzter Verbrauch: %.2f %s/Tag\n", *out.LastPerDay, t.Unit)
			} else {
				_, _ = fmt.Fprintln(w, "Letzter Verbrauch: keine Daten")
			}
			if out.TankRemaining != nil {
				_, _ = fmt.Fprintf(w, "Tankfüllstand: %.1f %s\n", *out.TankRemaining, t.Unit)
			}
			if out.DaysUntilEmpty != nil {
				_, _ = fmt.Fprintf(w, "Reichweite: %.1f Tage\n", *out.DaysUntilEmpty)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
```

In `internal/cli/root.go`, register the three new commands after the existing `root.AddCommand(...)` calls:
```go
	root.AddCommand(newReportCmd(app))
	root.AddCommand(newForecastCmd(app))
	root.AddCommand(newStatusCmd(app))
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/cli/... -v`
Expected: PASS for every test in the package (all tasks 11-13 combined).

- [ ] **Step 7: Commit**

```bash
git add internal/cli/report.go internal/cli/forecast.go internal/cli/status.go internal/cli/report_test.go internal/cli/root.go
git commit -m "feat: add report, forecast, and status commands"
```

---

## Task 14: Main Entry Point Wiring

**Files:**
- Modify: `cmd/verbrauch/main.go` (full replacement of the Task 1 stub)

**Interfaces:**
- Consumes: `cli.NewRootCmd`, `cli.UsageError` (Task 11)

- [ ] **Step 1: Replace `main.go` with full wiring**

`cmd/verbrauch/main.go`:
```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/maltekiefer/verbrauch/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCmd()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)

		var usageErr *cli.UsageError
		if errors.As(err, &usageErr) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build the binary**

Run:
```bash
go build -o verbrauch ./cmd/verbrauch
```
Expected: builds with no errors.

- [ ] **Step 3: Manual smoke test of the full flow**

Run (adjust for your shell; this is bash syntax):
```bash
export VERBRAUCH_DB=/tmp/verbrauch-smoke-test.db
./verbrauch type add oel --unit L --price 0.95 --tank-size 3000
./verbrauch type list
./verbrauch add --type oel --value 45000 --date 2026-08-01 --heating both
./verbrauch add --type oel --value 45120 --date 2026-08-15 --heating both
./verbrauch refill --type oel --amount 3000 --date 2026-08-01
./verbrauch report --type oel --period 30d
./verbrauch forecast --type oel --horizon 30d
./verbrauch status --type oel
rm /tmp/verbrauch-smoke-test.db
```
Expected: every command exits 0, `report` prints a table and an ASCII bar chart, `forecast` prints an average/day plus a cost line (price is set) and a days-until-empty line (tank size and a refill are set), `status` prints the same tank/reichweite figures.

Also verify the documented exit-code deviation:
```bash
./verbrauch add --type oel --value 1 --heating invalid; echo "exit=$?"
```
Expected: `exit=2`.

```bash
./verbrauch add --type does-not-exist --value 1; echo "exit=$?"
```
Expected: `exit=2`.

- [ ] **Step 4: Run the full test suite and static analysis**

Run:
```bash
go vet ./...
go test ./...
```
Expected: `go vet` reports nothing, all tests PASS. If `staticcheck` and/or `golangci-lint` are installed, also run:
```bash
staticcheck ./...
golangci-lint run
```
Fix any findings before proceeding.

- [ ] **Step 5: Commit**

```bash
git add cmd/verbrauch/main.go
git commit -m "feat: wire main entry point with signal handling and exit codes"
```

---

## Self-Review Notes

- **Spec coverage:** every spec section has a task — schema (Task 1/2), type CRUD (Task 3/11), readings (Task 4/12), refills (Task 5/12), diff calc (Task 6), aggregation (Task 7), chart (Task 8), forecast/cost/tank range (Task 9/10/13), CLI commands (Tasks 11-13), pragmas/signal handling/exit codes (Task 2/14).
- **Known simplification vs. spec text:** the spec's generic "Konfigurationsreihenfolge: Flags > Umgebungsvariablen > Konfigdatei > Defaults" is implemented without an actual config file layer in v1, documented in Global Constraints — there are only two global settings and both already have flag/env coverage.
- **Known simplification vs. user's general CLI standard:** exit code 2 only covers CLI-layer-detected usage errors (`cli.UsageError`), not cobra's own internal flag-parsing errors, documented in Global Constraints and called out again in Task 14.
- **Type consistency check performed:** `store.Reading.HeatingMode` (`sql.NullString`) → `report.ReadingInput.HeatingMode` (`string`) conversion happens once, in `toReadingInputs` (Task 13), and every command that needs it (`report`, `forecast`, `status`) calls that same helper rather than re-implementing the conversion.
