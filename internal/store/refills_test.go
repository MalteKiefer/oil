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
