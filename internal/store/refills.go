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
