package report_test

import (
	"strings"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/report"
)

func TestRenderChart(t *testing.T) {
	buckets := []report.Bucket{
		{Label: "2026-01-01", Total: 10},
		{Label: "2026-01-02", Total: 20},
	}

	got := report.RenderChart(buckets)

	if !strings.Contains(got, "2026-01-01") || !strings.Contains(got, "2026-01-02") {
		t.Fatalf("chart missing labels: %q", got)
	}
	if !strings.Contains(got, strings.Repeat("█", 20)) {
		t.Errorf("chart missing full-width (40-wide max) bar for the larger value: %q", got)
	}
	if !strings.Contains(got, strings.Repeat("█", 40)) {
		t.Errorf("chart missing full 40-char bar for the max value: %q", got)
	}
	if !strings.Contains(got, "10.0") || !strings.Contains(got, "20.0") {
		t.Errorf("chart missing numeric totals: %q", got)
	}
}

func TestRenderChart_Empty(t *testing.T) {
	got := report.RenderChart(nil)
	if !strings.Contains(got, "keine Daten") {
		t.Errorf("RenderChart(nil) = %q, want a no-data message", got)
	}
}
