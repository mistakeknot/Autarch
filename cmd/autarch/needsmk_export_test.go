package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNeedsMkExportCommand(t *testing.T) {
	cmd := needsMkCmd()
	export, _, err := cmd.Find([]string{"export"})
	if err != nil || export.Name() != "export" {
		t.Fatalf("export not registered: %v", err)
	}
	if export.Flags().Lookup("days").DefValue != "7" {
		t.Fatal("default window is not seven days")
	}
	cmd.SetArgs([]string{"export", "--days=0"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--days") {
		t.Fatalf("expected days validation, got %v", err)
	}
}
