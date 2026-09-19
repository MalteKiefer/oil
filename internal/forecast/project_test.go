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
