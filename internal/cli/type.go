package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func newTypeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "type",
		Short: "Verbrauchsarten verwalten",
	}
	cmd.AddCommand(newTypeAddCmd(app))
	cmd.AddCommand(newTypeListCmd(app))
	return cmd
}

func newTypeAddCmd(app *App) *cobra.Command {
	var unit string
	var price, tankSize float64

	cmd := &cobra.Command{
		Use:     "add <name>",
		Short:   "Neue Verbrauchsart anlegen",
		Args:    cobra.ExactArgs(1),
		Example: "  verbrauch type add oel --unit L --price 0.95 --tank-size 3000",
		RunE: func(cmd *cobra.Command, args []string) error {
			t := store.ConsumptionType{Name: args[0], Unit: unit}
			if cmd.Flags().Changed("price") {
				t.PricePerUnit = sql.NullFloat64{Float64: price, Valid: true}
			}
			if cmd.Flags().Changed("tank-size") {
				t.TankSize = sql.NullFloat64{Float64: tankSize, Valid: true}
			}

			db, err := app.openDB(cmd)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			id, err := db.CreateType(cmd.Context(), t)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verbrauchsart %q angelegt (id=%d)\n", t.Name, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "Einheit, z.B. L oder kWh")
	_ = cmd.MarkFlagRequired("unit")
	cmd.Flags().Float64Var(&price, "price", 0, "Preis pro Einheit (optional)")
	cmd.Flags().Float64Var(&tankSize, "tank-size", 0, "Tankgröße für Reichweitenberechnung (optional)")
	return cmd
}

type typeDTO struct {
	Name         string   `json:"name"`
	Unit         string   `json:"unit"`
	PricePerUnit *float64 `json:"price_per_unit,omitempty"`
	TankSize     *float64 `json:"tank_size,omitempty"`
}

func toTypeDTO(t store.ConsumptionType) typeDTO {
	dto := typeDTO{Name: t.Name, Unit: t.Unit}
	if t.PricePerUnit.Valid {
		dto.PricePerUnit = &t.PricePerUnit.Float64
	}
	if t.TankSize.Valid {
		dto.TankSize = &t.TankSize.Float64
	}
	return dto
}

func newTypeListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "Alle Verbrauchsarten auflisten",
		Example: "  verbrauch type list",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := app.openDB(cmd)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			types, err := db.ListTypes(cmd.Context())
			if err != nil {
				return err
			}

			if app.json {
				dtos := make([]typeDTO, len(types))
				for i, t := range types {
					dtos[i] = toTypeDTO(t)
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(dtos)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tEINHEIT\tPREIS\tTANKGRÖSSE")
			for _, t := range types {
				price, tank := "-", "-"
				if t.PricePerUnit.Valid {
					price = fmt.Sprintf("%.2f", t.PricePerUnit.Float64)
				}
				if t.TankSize.Valid {
					tank = fmt.Sprintf("%.0f", t.TankSize.Float64)
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, t.Unit, price, tank)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("ausgabe schreiben: %w", err)
			}
			return nil
		},
	}
}
