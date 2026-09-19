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

func toRefillInputs(refills []store.Refill) []forecast.RefillInput {
	inputs := make([]forecast.RefillInput, len(refills))
	for i, r := range refills {
		d, _ := time.Parse("2006-01-02", r.RefillDate)
		inputs[i] = forecast.RefillInput{Date: d, Amount: r.Amount}
	}
	return inputs
}

func parseHorizonDays(horizon string) (int, error) {
	switch horizon {
	case "7d":
		return 7, nil
	case "30d":
		return 30, nil
	default:
		return 0, newUsageError("ungültiger --horizon Wert %q, erlaubt: 7d, 30d", horizon)
	}
}

func newForecastCmd(app *App) *cobra.Command {
	var typeName, horizon string

	cmd := &cobra.Command{
		Use:     "forecast",
		Short:   "Verbrauchs-, Kosten- und Reichweitenprognose",
		Example: "  verbrauch forecast --type oel --horizon 30d",
		RunE: func(cmd *cobra.Command, args []string) error {
			horizonDays, err := parseHorizonDays(horizon)
			if err != nil {
				return err
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

			// .UTC() matters here for the same reason as in report.go: ComputeTrend
			// normalizes "now" to a UTC calendar-day midnight internally.
			trend, err := forecast.ComputeTrend(points, 30, time.Now().UTC())
			if err != nil {
				return err
			}

			var pricePtr *float64
			if t.PricePerUnit.Valid {
				pricePtr = &t.PricePerUnit.Float64
			}

			var tankRemainingPtr *float64
			if t.TankSize.Valid {
				refills, err := db.ListRefills(cmd.Context(), t.ID)
				if err != nil {
					return err
				}
				if remaining, ok := forecast.TankRemaining(toRefillInputs(refills), points); ok {
					tankRemainingPtr = &remaining
				}
			}

			projection := forecast.Project(trend, horizonDays, pricePtr, tankRemainingPtr)

			if app.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(projection)
			}

			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Durchschnitt: %.2f %s/Tag\n", trend.AvgPerDay, t.Unit)
			_, _ = fmt.Fprintf(out, "Prognose (%d Tage): %.1f %s\n", projection.HorizonDays, projection.Projected, t.Unit)
			if projection.Cost != nil {
				_, _ = fmt.Fprintf(out, "Kostenprognose: %.2f\n", *projection.Cost)
			}
			if projection.DaysUntilEmpty != nil {
				_, _ = fmt.Fprintf(out, "Tage bis Tank leer: %.1f\n", *projection.DaysUntilEmpty)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().StringVar(&horizon, "horizon", "30d", "Prognosehorizont: 7d oder 30d")
	return cmd
}
