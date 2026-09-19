package cli_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/cli"
)

func setupType(t *testing.T, dbPath, name, unit string) {
	t.Helper()
	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "type", "add", name, "--unit", unit})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("setupType(%q): %v", name, err)
	}
}

func TestAddReading(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", "oel", "--value", "45231.5", "--date", "2026-09-19", "--heating", "both"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out.String(), "erfasst") {
		t.Errorf("add output = %q, want confirmation containing 'erfasst'", out.String())
	}
}

func TestAddReading_InvalidHeatingIsUsageError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")

	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", "oel", "--value", "1", "--heating", "invalid"})
	err := cmd.Execute()
	var usageErr *cli.UsageError
	if err == nil {
		t.Fatal("add with invalid --heating: want error, got nil")
	}
	if !errors.As(err, &usageErr) {
		t.Errorf("add with invalid --heating: error = %v, want *cli.UsageError", err)
	}
}

func TestAddReading_UnknownTypeIsUsageError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	cmd := cli.NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "add", "--type", "does-not-exist", "--value", "1"})
	err := cmd.Execute()
	var usageErr *cli.UsageError
	if err == nil {
		t.Fatal("add with unknown --type: want error, got nil")
	}
	if !errors.As(err, &usageErr) {
		t.Errorf("add with unknown --type: error = %v, want *cli.UsageError", err)
	}
}

func TestRefill(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")
	setupType(t, dbPath, "oel", "L")

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--db", dbPath, "refill", "--type", "oel", "--amount", "2500", "--date", "2026-09-19"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("refill: %v", err)
	}
	if !strings.Contains(out.String(), "erfasst") {
		t.Errorf("refill output = %q, want confirmation containing 'erfasst'", out.String())
	}
}
