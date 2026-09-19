package forecast

import (
	"errors"
	"time"

	"github.com/maltekiefer/verbrauch/internal/report"
)

var ErrInsufficientData = errors.New("zu wenig Daten für Prognose")

// Trend is the average consumption per day over a lookback window.
type Trend struct {
	AvgPerDay float64
}

// ComputeTrend averages consumption per day over the windowDays before now,
// using only points with a valid (non-skipped, non-zero) diff. It returns
// ErrInsufficientData if no such point falls in the window.
func ComputeTrend(points []report.DailyPoint, windowDays int, now time.Time) (Trend, error) {
	cutoff := now.AddDate(0, 0, -windowDays)

	var totalDiff float64
	var totalDays int

	for _, p := range points {
		if p.Skipped || p.DiffDays == 0 {
			continue
		}
		if p.Date.Before(cutoff) {
			continue
		}
		totalDiff += p.Diff
		totalDays += p.DiffDays
	}

	if totalDays == 0 {
		return Trend{}, ErrInsufficientData
	}
	return Trend{AvgPerDay: totalDiff / float64(totalDays)}, nil
}
