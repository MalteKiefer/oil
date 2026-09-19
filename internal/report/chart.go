package report

import (
	"fmt"
	"strings"
)

const chartWidth = 40

// RenderChart draws a horizontal ASCII/Unicode bar chart, one line per
// bucket, scaled so the largest total fills chartWidth characters.
func RenderChart(buckets []Bucket) string {
	if len(buckets) == 0 {
		return "(keine Daten für Diagramm)\n"
	}

	max := 0.0
	for _, b := range buckets {
		if b.Total > max {
			max = b.Total
		}
	}

	var sb strings.Builder
	for _, b := range buckets {
		barLen := 0
		if max > 0 {
			barLen = int(b.Total / max * chartWidth)
		}
		bar := strings.Repeat("█", barLen)
		fmt.Fprintf(&sb, "%-10s %s %.1f\n", b.Label, bar, b.Total)
	}
	return sb.String()
}
