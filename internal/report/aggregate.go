package report

import (
	"fmt"
	"sort"
	"time"
)

// Bucket is one aggregated slot in a report (a day, a week, or a month).
type Bucket struct {
	Label string
	Total float64
}

type granularity int

const (
	granDay granularity = iota
	granWeek
	granMonth
)

func periodBounds(period string, now time.Time) (time.Time, granularity, error) {
	switch period {
	case "7d":
		return now.AddDate(0, 0, -7), granDay, nil
	case "30d":
		return now.AddDate(0, 0, -30), granDay, nil
	case "6m":
		return now.AddDate(0, -6, 0), granWeek, nil
	case "12m":
		return now.AddDate(-1, 0, 0), granMonth, nil
	default:
		return time.Time{}, 0, fmt.Errorf("unbekannte periode %q, erlaubt: 7d, 30d, 6m, 12m", period)
	}
}

func bucketKey(t time.Time, g granularity) string {
	switch g {
	case granWeek:
		year, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case granMonth:
		return t.Format("2006-01")
	default:
		return t.Format("2006-01-02")
	}
}

// AggregateByPeriod buckets the consumption in points into the given period's
// granularity (7d/30d: daily, 6m: weekly, 12m: monthly), restricted to points
// on or after now minus the period. Each point's Diff is spread evenly across
// the DiffDays days it covers before bucketing, so a reading taken every few
// days still produces a smooth daily/weekly/monthly series.
func AggregateByPeriod(points []DailyPoint, period string, now time.Time) ([]Bucket, error) {
	cutoff, gran, err := periodBounds(period, now)
	if err != nil {
		return nil, err
	}

	totals := map[string]float64{}
	seen := map[string]bool{}
	var order []string

	for _, p := range points {
		if p.Skipped || p.DiffDays == 0 {
			continue
		}
		perDay := p.Diff / float64(p.DiffDays)
		for d := 0; d < p.DiffDays; d++ {
			day := p.Date.AddDate(0, 0, -d)
			if day.Before(cutoff) {
				continue
			}
			key := bucketKey(day, gran)
			if !seen[key] {
				seen[key] = true
				order = append(order, key)
			}
			totals[key] += perDay
		}
	}

	sort.Strings(order)
	buckets := make([]Bucket, 0, len(order))
	for _, key := range order {
		buckets = append(buckets, Bucket{Label: key, Total: totals[key]})
	}
	return buckets, nil
}
