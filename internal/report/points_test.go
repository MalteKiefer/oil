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
