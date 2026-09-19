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
