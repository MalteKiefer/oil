package forecast

import (
	"time"

	"github.com/maltekiefer/verbrauch/internal/report"
)

// RefillInput is a store-independent view of one tank refill event.
type RefillInput struct {
	Date   time.Time
	Amount float64
}

// TankRemaining computes the estimated amount left in the tank, based on the
// most recent refill minus every valid diff on or after that refill's date.
// It returns ok=false if there are no refills to anchor the calculation to.
func TankRemaining(refills []RefillInput, points []report.DailyPoint) (float64, bool) {
	if len(refills) == 0 {
		return 0, false
	}

	last := refills[len(refills)-1]
	var consumed float64
	for _, p := range points {
		if p.Skipped || p.DiffDays == 0 {
			continue
		}
		if p.Date.After(last.Date) {
			consumed += p.Diff
		}
	}
	return last.Amount - consumed, true
}

// Projection is the forecast result for a given horizon.
type Projection struct {
	HorizonDays    int
	Projected      float64
	Cost           *float64
	DaysUntilEmpty *float64
}

// Project extrapolates trend.AvgPerDay over horizonDays, optionally adding a
// cost projection (if pricePerUnit is given) and a days-until-empty estimate
// (if tankRemaining is given and the trend is positive).
func Project(trend Trend, horizonDays int, pricePerUnit *float64, tankRemaining *float64) Projection {
	proj := Projection{
		HorizonDays: horizonDays,
		Projected:   trend.AvgPerDay * float64(horizonDays),
	}

	if pricePerUnit != nil {
		cost := proj.Projected * *pricePerUnit
		proj.Cost = &cost
	}

	if tankRemaining != nil && trend.AvgPerDay > 0 {
		days := *tankRemaining / trend.AvgPerDay
		proj.DaysUntilEmpty = &days
	}

	return proj
}
