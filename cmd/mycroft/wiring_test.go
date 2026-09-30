package main

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/spawn"
)

func TestRunPathSetsDecisionQueue(t *testing.T) {
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	o := buildOrchestrator(db, spawn.NewClaudeCodeSpawner("", "Demarch"), mycroft.DefaultConfig())
	if !o.HasQueue() {
		t.Fatal("production orchestrator has no decision queue")
	}
}
