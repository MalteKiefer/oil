package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/forecast"
	"github.com/maltekiefer/verbrauch/internal/report"
	"github.com/maltekiefer/verbrauch/internal/store"
)

type statusOutput struct {
	Type           string   `json:"type"`
	LastPerDay     *float64 `json:"last_per_day,omitempty"`
	TankRemaining  *float64 `json:"tank_remaining,omitempty"`
	DaysUntilEmpty *float64 `json:"days_until_empty,omitempty"`
}

func newStatusCmd(app *App) *cobra.Command {
	var typeName string

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Kurzübersicht: Füllstand, Reichweite, letzter Verbrauch",
		Example: "  verbrauch status --type oel",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			out := statusOutput{Type: t.Name}
			if len(points) > 0 {
				last := points[len(points)-1]
				if !last.Skipped && last.DiffDays > 0 {
					perDay := last.Diff / float64(last.DiffDays)
					out.LastPerDay = &perDay
				}
			}

			if t.TankSize.Valid {
				refills, err := db.ListRefills(cmd.Context(), t.ID)
				if err != nil {
					return err
				}
				if remaining, ok := forecast.TankRemaining(toRefillInputs(refills), points); ok {
					out.TankRemaining = &remaining
					// .UTC() matters here for the same reason as report.go/forecast.go.
					if trend, err := forecast.ComputeTrend(points, 30, time.Now().UTC()); err == nil && trend.AvgPerDay > 0 {
						days := remaining / trend.AvgPerDay
						out.DaysUntilEmpty = &days
					}
				}
			}

			if app.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
			}

			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "Verbrauchsart: %s\n", out.Type)
			if out.LastPerDay != nil {
				_, _ = fmt.Fprintf(w, "Letzter Verbrauch: %.2f %s/Tag\n", *out.LastPerDay, t.Unit)
			} else {
				_, _ = fmt.Fprintln(w, "Letzter Verbrauch: keine Daten")
			}
			if out.TankRemaining != nil {
				_, _ = fmt.Fprintf(w, "Tankfüllstand: %.1f %s\n", *out.TankRemaining, t.Unit)
			}
			if out.DaysUntilEmpty != nil {
				_, _ = fmt.Fprintf(w, "Reichweite: %.1f Tage\n", *out.DaysUntilEmpty)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
