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
