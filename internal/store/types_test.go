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
