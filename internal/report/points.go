package report

import (
	"fmt"
	"time"
)

// ReadingInput is a store-independent view of one meter reading, used as
// input to BuildPoints. Callers must pass readings sorted ascending by Date.
type ReadingInput struct {
	Date         time.Time
	CounterValue float64
	HeatingMode  string // "" if not set
}

// DailyPoint is one reading annotated with the consumption since the
// previous reading. Diff/DiffDays are zero for the first point of a series
// and for any point whose computed diff was negative (Skipped = true).
type DailyPoint struct {
	Date         time.Time
	CounterValue float64
	HeatingMode  string
	Diff         float64
	DiffDays     int
	Skipped      bool
}

// BuildPoints turns a sorted series of readings into DailyPoints, computing
// the consumption between each reading and its immediate predecessor. A
// negative diff (counter went down — meter swap or typo) is reported as a
// warning and excluded from Diff/DiffDays on that point.
func BuildPoints(readings []ReadingInput) ([]DailyPoint, []string) {
	var points []DailyPoint
	var warnings []string

	for i, r := range readings {
		p := DailyPoint{
			Date:         r.Date,
			CounterValue: r.CounterValue,
			HeatingMode:  r.HeatingMode,
		}
		if i == 0 {
			points = append(points, p)
			continue
		}

		prev := readings[i-1]
		diff := r.CounterValue - prev.CounterValue
		if diff < 0 {
			warnings = append(warnings, fmt.Sprintf(
				"negativer Verbrauch am %s übersprungen (Zählerstand gesunken von %.2f auf %.2f)",
				r.Date.Format("2006-01-02"), prev.CounterValue, r.CounterValue))
			p.Skipped = true
			points = append(points, p)
			continue
		}

		p.Diff = diff
		p.DiffDays = int(r.Date.Sub(prev.Date).Hours() / 24)
		points = append(points, p)
	}

	return points, warnings
}
