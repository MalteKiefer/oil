package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/maltekiefer/verbrauch/internal/store"
)

// UsageError marks an error caused by invalid user input (a bad flag value,
// an unknown type name, ...), as opposed to an internal/runtime failure.
// main.go maps this to exit code 2.
type UsageError struct {
	msg string
}

func (e *UsageError) Error() string { return e.msg }

func newUsageError(format string, args ...any) error {
	return &UsageError{msg: fmt.Sprintf(format, args...)}
}

// App holds the flags shared across every command invocation. It does not
// hold a *store.DB — see openDB below for why.
type App struct {
	dbPath string
	json   bool
}

// openDB resolves the database path and opens a fresh connection for one
// command invocation. Every RunE calls this at the top and defers Close on
// the result:
//
//	db, err := app.openDB(cmd)
//	if err != nil {
//		return err
//	}
//	defer func() { _ = db.Close() }()
//
// Deferring inside RunE itself guarantees the connection closes whether
// RunE returns an error or not. Do not move this into a
// PersistentPreRunE/PersistentPostRunE pair: cobra skips
// PersistentPostRunE whenever RunE returns a non-nil error, which would
// leak the connection on every error path.
func (app *App) openDB(cmd *cobra.Command) (*store.DB, error) {
	path, err := resolveDBPath(app.dbPath)
	if err != nil {
		return nil, err
	}
	return store.Open(cmd.Context(), path)
}

// NewRootCmd builds the verbrauch command tree.
func NewRootCmd() *cobra.Command {
	app := &App{}

	root := &cobra.Command{
		Use:   "verbrauch",
		Short: "Erfasst und wertet Verbrauchszählerstände aus (Öl, Strom, ...)",
	}
	root.SilenceUsage = true
	root.SilenceErrors = true

	root.PersistentFlags().StringVar(&app.dbPath, "db", "",
		"Pfad zur SQLite-Datenbank (Default: plattformspezifischer Konfigordner)")
	root.PersistentFlags().BoolVar(&app.json, "json", false, "Ausgabe als JSON")

	root.AddCommand(newTypeCmd(app))

	return root
}

func resolveDBPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if envPath := os.Getenv("VERBRAUCH_DB"); envPath != "" {
		return envPath, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("konfigverzeichnis ermitteln: %w", err)
	}
	return filepath.Join(configDir, "verbrauch", "verbrauch.db"), nil
}
