package cli_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/cli"
)

func TestTypeAddAndList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	addCmd := cli.NewRootCmd()
	addCmd.SetOut(io.Discard)
	addCmd.SetErr(io.Discard)
	addCmd.SetArgs([]string{"--db", dbPath, "type", "add", "oel", "--unit", "L", "--price", "0.95", "--tank-size", "3000"})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("type add: %v", err)
	}

	var out bytes.Buffer
	listCmd := cli.NewRootCmd()
	listCmd.SetOut(&out)
	listCmd.SetErr(io.Discard)
	listCmd.SetArgs([]string{"--db", dbPath, "type", "list"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("type list: %v", err)
	}

	got := out.String()
	for _, want := range []string{"oel", "L", "0.95", "3000"} {
		if !strings.Contains(got, want) {
			t.Errorf("type list output %q missing %q", got, want)
		}
	}
}

func TestTypeAdd_DuplicateIsUsageStyleError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	first := cli.NewRootCmd()
	first.SetOut(io.Discard)
	first.SetErr(io.Discard)
	first.SetArgs([]string{"--db", dbPath, "type", "add", "oel", "--unit", "L"})
	if err := first.Execute(); err != nil {
		t.Fatalf("first type add: %v", err)
	}

	second := cli.NewRootCmd()
	second.SetOut(io.Discard)
	second.SetErr(io.Discard)
	second.SetArgs([]string{"--db", dbPath, "type", "add", "oel", "--unit", "L"})
	if err := second.Execute(); err == nil {
		t.Fatal("second type add: want error for duplicate name, got nil")
	}
}
