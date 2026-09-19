package cli_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maltekiefer/verbrauch/internal/cli"
)

func addReading(t *testing.T, dbPath, typeName, date string, value float64) {
	t.Helper()
	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", typeName, "--value", floatStr(value), "--date", date})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("addReading(%s, %v): %v", date, value, err)
	}
}

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func TestReportAndForecastAndStatus(t *testing.T) {
	// Dates are anchored to "today" (not hardcoded absolute dates) because
	// the forecast/status RunEs pass time.Now().UTC() into
	// forecast.ComputeTrend, which only looks at a 30-day window before
	// now. Hardcoded past dates would drift out of that window and start
	// failing with ErrInsufficientData once enough real time has passed.
	// The gaps (6, then 3 days) mirror the original fixture's shape.
	today := time.Now().UTC()
	date1 := today.AddDate(0, 0, -9).Format("2006-01-02")
	date2 := today.AddDate(0, 0, -3).Format("2006-01-02")
	date3 := today.Format("2006-01-02")

	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")
	addReading(t, dbPath, "oel", date1, 100)
	addReading(t, dbPath, "oel", date2, 106)
	addReading(t, dbPath, "oel", date3, 112)

	var reportOut bytes.Buffer
	reportCmd := cli.NewRootCmd()
	reportCmd.SetOut(&reportOut)
	reportCmd.SetErr(io.Discard)
	reportCmd.SetArgs([]string{"--db", dbPath, "report", "--type", "oel", "--period", "30d"})
	if err := reportCmd.Execute(); err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(reportOut.String(), "ZEITRAUM") {
		t.Errorf("report output = %q, want a table header", reportOut.String())
	}

	var forecastOut bytes.Buffer
	forecastCmd := cli.NewRootCmd()
	forecastCmd.SetOut(&forecastOut)
	forecastCmd.SetErr(io.Discard)
	forecastCmd.SetArgs([]string{"--db", dbPath, "forecast", "--type", "oel", "--horizon", "7d"})
	if err := forecastCmd.Execute(); err != nil {
		t.Fatalf("forecast: %v", err)
	}
	if !strings.Contains(forecastOut.String(), "Prognose") {
		t.Errorf("forecast output = %q, want a Prognose line", forecastOut.String())
	}

	var statusOut bytes.Buffer
	statusCmd := cli.NewRootCmd()
	statusCmd.SetOut(&statusOut)
	statusCmd.SetErr(io.Discard)
	statusCmd.SetArgs([]string{"--db", dbPath, "status", "--type", "oel"})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(statusOut.String(), "oel") {
		t.Errorf("status output = %q, want it to mention the type name", statusOut.String())
	}
}

func TestReportAndForecast_JSONUsesSnakeCaseKeys(t *testing.T) {
	// Anchored to "today" (see TestReportAndForecastAndStatus above) since
	// forecast.ComputeTrend only looks at a 30-day window before now, and
	// needs at least 2 valid diffs (3 readings) to avoid ErrInsufficientData.
	today := time.Now().UTC()
	date1 := today.AddDate(0, 0, -9).Format("2006-01-02")
	date2 := today.AddDate(0, 0, -3).Format("2006-01-02")
	date3 := today.Format("2006-01-02")

	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")
	addReading(t, dbPath, "oel", date1, 100)
	addReading(t, dbPath, "oel", date2, 106)
	addReading(t, dbPath, "oel", date3, 112)

	tests := []struct {
		name        string
		args        []string
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "report",
			args:        []string{"--db", dbPath, "--json", "report", "--type", "oel", "--period", "30d"},
			wantContain: []string{`"readings"`, `"buckets"`, `"date"`, `"counter_value"`},
			wantAbsent:  []string{`"Label"`, `"Total"`, `"CounterValue"`},
		},
		{
			name:        "forecast",
			args:        []string{"--db", dbPath, "--json", "forecast", "--type", "oel", "--horizon", "7d"},
			wantContain: []string{`"horizon_days"`, `"projected"`},
			wantAbsent:  []string{`"HorizonDays"`, `"Projected"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := cli.NewRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			got := out.String()
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("%s JSON output %q missing %q", tt.name, got, want)
				}
			}
			for _, notWant := range tt.wantAbsent {
				if strings.Contains(got, notWant) {
					t.Errorf("%s JSON output %q unexpectedly contains PascalCase key %q", tt.name, got, notWant)
				}
			}
		})
	}
}

func TestForecast_InvalidHorizonIsUsageError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")
	addReading(t, dbPath, "oel", "2026-01-01", 100)
	addReading(t, dbPath, "oel", "2026-01-03", 106)

	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "forecast", "--type", "oel", "--horizon", "1y"})
	err := cmd.Execute()
	var usageErr *cli.UsageError
	if err == nil {
		t.Fatal("forecast with invalid --horizon: want error, got nil")
	}
	if !errors.As(err, &usageErr) {
		t.Errorf("forecast with invalid --horizon: error = %v, want *cli.UsageError", err)
	}
}
