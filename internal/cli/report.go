package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/report"
	"github.com/maltekiefer/verbrauch/internal/store"
)

func toReadingInputs(readings []store.Reading) []report.ReadingInput {
	inputs := make([]report.ReadingInput, len(readings))
	for i, r := range readings {
		d, _ := time.Parse("2006-01-02", r.ReadingDate)
		mode := ""
		if r.HeatingMode.Valid {
			mode = r.HeatingMode.String
		}
		inputs[i] = report.ReadingInput{Date: d, CounterValue: r.CounterValue, HeatingMode: mode}
	}
	return inputs
}

var validPeriods = map[string]bool{"7d": true, "30d": true, "6m": true, "12m": true}

func newReportCmd(app *App) *cobra.Command {
	var typeName, period string

	cmd := &cobra.Command{
		Use:     "report",
		Short:   "Verbrauchsbericht (Tabelle + Diagramm)",
		Example: "  verbrauch report --type oel --period 30d",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validPeriods[period] {
				return newUsageError("ungültiger --period Wert %q, erlaubt: 7d, 30d, 6m, 12m", period)
			}

			db, err := app.openDB(cmd)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			t, err := db.GetTypeByName(cmd.Context(), typeName)
			if err != nil {
				if errors.Is(err, store.ErrTypeNotFound) {
					return newUsageError("Verbrauchsart %q nicht gefunden, siehe 'verbrauch type list'", typeName)
				}
				return err
			}

			readings, err := db.ListReadings(cmd.Context(), t.ID)
			if err != nil {
				return err
			}
			points, warnings := report.BuildPoints(toReadingInputs(readings))
			for _, w := range warnings {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warnung:", w)
			}

			// .UTC() matters here: AggregateByPeriod normalizes "now" to a UTC
			// calendar-day midnight internally (see internal/report/aggregate.go);
			// passing a non-UTC time.Now() would still work today because that
			// normalization only looks at Year/Month/Day, but future code that
			// forgets that assumption would silently apply the wrong local day.
			// Being explicit here is cheap insurance.
			buckets, err := report.AggregateByPeriod(points, period, time.Now().UTC())
			if err != nil {
				return newUsageError("%s", err.Error())
			}

			if app.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(buckets)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintf(w, "ZEITRAUM\tVERBRAUCH (%s)\n", t.Unit)
			for _, b := range buckets {
				_, _ = fmt.Fprintf(w, "%s\t%.1f\n", b.Label, b.Total)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("ausgabe schreiben: %w", err)
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprint(cmd.OutOrStdout(), report.RenderChart(buckets))
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().StringVar(&period, "period", "30d", "Zeitraum: 7d, 30d, 6m, 12m")
	return cmd
}
