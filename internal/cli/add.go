package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func newAddCmd(app *App) *cobra.Command {
	var typeName, dateStr, heating string
	var value float64

	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Zählerstand erfassen",
		Example: "  verbrauch add --type oel --value 45231.5 --heating both",
		RunE: func(cmd *cobra.Command, args []string) error {
			date := dateStr
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return newUsageError("ungültiges Datum %q, erwartet YYYY-MM-DD", date)
			}

			var mode sql.NullString
			switch heating {
			case "":
				// kein Wert
			case "off":
				mode = sql.NullString{String: "off", Valid: true}
			case "water":
				mode = sql.NullString{String: "water_only", Valid: true}
			case "both":
				mode = sql.NullString{String: "water_and_heat", Valid: true}
			default:
				return newUsageError("ungültiger --heating Wert %q, erlaubt: off, water, both", heating)
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

			id, err := db.InsertReading(cmd.Context(), store.Reading{
				TypeID:       t.ID,
				ReadingDate:  date,
				CounterValue: value,
				HeatingMode:  mode,
				CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			if app.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"id": id})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Zählerstand erfasst (id=%d)\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart (siehe 'verbrauch type list')")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().Float64Var(&value, "value", 0, "Zählerstand")
	_ = cmd.MarkFlagRequired("value")
	cmd.Flags().StringVar(&dateStr, "date", "", "Datum YYYY-MM-DD (Default: heute)")
	cmd.Flags().StringVar(&heating, "heating", "", "Heizmodus: off, water, both (optional)")
	return cmd
}
