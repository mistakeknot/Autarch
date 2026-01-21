package storage

import "testing"

func TestAgentUpsertAndGet(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	created, err := UpsertAgent(db, AgentProfile{
		Name:            "BlueLake",
		Program:         "codex-cli",
		Model:           "gpt-5",
		TaskDescription: "Auth refactor",
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if created.Name != "BlueLake" || created.CreatedAt == "" || created.LastActiveAt == "" {
		t.Fatalf("expected created agent")
	}

	agent, err := GetAgent(db, "BlueLake")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if agent.Program != "codex-cli" || agent.Model != "gpt-5" {
		t.Fatalf("unexpected agent data")
	}
	if agent.TaskDescription != "Auth refactor" {
		t.Fatalf("unexpected task description")
	}
}
