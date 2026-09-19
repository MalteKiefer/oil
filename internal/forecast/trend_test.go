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

func TestComputeTrend_NonMidnightNow(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2026-01-14"), CounterValue: 0},
		{Date: date("2026-01-16"), CounterValue: 4}, // Diff=4, DiffDays=2
	}
	points, _ := report.BuildPoints(readings)

	// now carries a non-midnight time-of-day on purpose: this is what a real
	// time.Now() caller (Task 13's CLI) will pass. now - 30 days = 2026-01-16
	// (the exact date of the second reading). Without normalizing now to a
	// calendar-day midnight first, cutoff would be 2026-01-16T09:00:00, which
	// would incorrectly exclude the point dated 2026-01-16T00:00:00 (it comes
	// "before" that cutoff), leaving zero valid points and wrongly returning
	// ErrInsufficientData. With the fix, cutoff is 2026-01-16T00:00:00, so the
	// point is correctly included ("on/after the window cutoff").
	now := date("2026-02-15").Add(9 * time.Hour)

	trend, err := forecast.ComputeTrend(points, 30, now)
	if err != nil {
		t.Fatalf("ComputeTrend() error = %v, want nil (boundary point should be included)", err)
	}
	want := 2.0 // Diff=4 / DiffDays=2
	if math.Abs(trend.AvgPerDay-want) > 0.001 {
		t.Errorf("AvgPerDay = %v, want %v", trend.AvgPerDay, want)
	}
}
