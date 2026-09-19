package report_test

import (
	"testing"
	"time"

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

func TestAggregateByPeriod_NonMidnightNow(t *testing.T) {
	readings := []report.ReadingInput{
		{Date: date("2025-12-31"), CounterValue: 0},
		{Date: date("2026-01-02"), CounterValue: 2}, // Diff=2, DiffDays=2 -> spreads to Jan 1 and Jan 2
	}
	points, _ := report.BuildPoints(readings)

	// now carries a non-midnight time-of-day on purpose: this is what a real
	// `time.Now()` caller (Task 13's CLI) will pass. Without normalizing now
	// to a calendar-day midnight first, cutoff would be 2026-01-01T18:30:00,
	// which would incorrectly exclude the Jan 1 bucket (00:00 < 18:30). With
	// the fix, cutoff is 2026-01-01T00:00:00, so Jan 1 is correctly included
	// ("on or after now-minus-period").
	now := date("2026-01-08").Add(18*time.Hour + 30*time.Minute)

	buckets, err := report.AggregateByPeriod(points, "7d", now)
	if err != nil {
		t.Fatalf("AggregateByPeriod() error = %v", err)
	}

	found := false
	for _, b := range buckets {
		if b.Label == "2026-01-01" {
			found = true
			if b.Total != 1 {
				t.Errorf("2026-01-01 bucket Total = %v, want 1", b.Total)
			}
		}
	}
	if !found {
		t.Fatalf("expected a 2026-01-01 bucket (boundary day should be included), got %+v", buckets)
	}
}
