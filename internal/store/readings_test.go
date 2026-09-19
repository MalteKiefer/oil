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
