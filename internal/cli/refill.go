package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func newRefillCmd(app *App) *cobra.Command {
	var typeName, dateStr string
	var amount float64

	cmd := &cobra.Command{
		Use:     "refill",
		Short:   "Tankbefüllung erfassen",
		Example: "  verbrauch refill --type oel --amount 2500",
		RunE: func(cmd *cobra.Command, args []string) error {
			date := dateStr
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return newUsageError("ungültiges Datum %q, erwartet YYYY-MM-DD", date)
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

			id, err := db.InsertRefill(cmd.Context(), store.Refill{
				TypeID:     t.ID,
				RefillDate: date,
				Amount:     amount,
				CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Befüllung erfasst (id=%d)\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "Verbrauchsart")
	_ = cmd.MarkFlagRequired("type")
	cmd.Flags().Float64Var(&amount, "amount", 0, "Nachgefüllte Menge")
	_ = cmd.MarkFlagRequired("amount")
	cmd.Flags().StringVar(&dateStr, "date", "", "Datum YYYY-MM-DD (Default: heute)")
	return cmd
}
