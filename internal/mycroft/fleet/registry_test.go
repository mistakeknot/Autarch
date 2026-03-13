package fleet

import (
	"os"
	"path/filepath"
	"testing"
)

const testRegistry = `
version: "1.0"

capability_vocabulary:
  - debugging
  - test_discipline

agents:
  grey-area:
    source: autarch
    category: orchestration
    description: "Fleet coordination agent"
    capabilities: [debugging, test_discipline]
    roles: [coordinator]
    runtime:
      mode: cli
      binary: mycroft
    models:
      preferred: sonnet
      supported: [sonnet, opus]
    tools: [intermux, interlock, beads, tmux]
    cold_start_tokens: 500
    tags: [orchestration]

  fd-architecture:
    source: interflux
    category: review
    description: "Architecture reviewer"
    capabilities: [domain_review]
    roles: [fd-architecture]
    runtime:
      mode: subagent
      subagent_type: "interflux:review:fd-architecture"
    models:
      preferred: sonnet
      supported: [haiku, sonnet, opus]
    tools: [Read, Grep]
    cold_start_tokens: 800
    tags: [technical]

  falling-outside:
    source: autarch
    category: implementation
    description: "General implementation agent"
    capabilities: [debugging]
    roles: [implementer]
    runtime:
      mode: cli
    models:
      preferred: opus
      supported: [sonnet, opus]
    tools: [Read, Edit, Bash]
    tags: [implementation, session]
`

func writeTestRegistry(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fleet-registry.yaml")
	if err := os.WriteFile(path, []byte(testRegistry), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRegistry(t *testing.T) {
	path := writeTestRegistry(t)
	specs, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(specs))
	}

	// Find grey-area.
	ga, ok := FindByName(specs, "grey-area")
	if !ok {
		t.Fatal("grey-area not found")
	}
	if ga.Runtime.Mode != "cli" {
		t.Errorf("grey-area runtime: got %q, want %q", ga.Runtime.Mode, "cli")
	}
	if ga.Models.Preferred != "sonnet" {
		t.Errorf("grey-area model: got %q, want %q", ga.Models.Preferred, "sonnet")
	}
	if len(ga.Capabilities) != 2 {
		t.Errorf("grey-area capabilities: got %d, want 2", len(ga.Capabilities))
	}
	if ga.ColdStartTokens != 500 {
		t.Errorf("grey-area cold_start_tokens: got %d, want 500", ga.ColdStartTokens)
	}
}

func TestLoadRegistryMissing(t *testing.T) {
	_, err := LoadRegistry("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadRegistryMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(path, []byte("not: [valid: yaml: {{"), 0644)
	_, err := LoadRegistry(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoadRegistryOrEmpty(t *testing.T) {
	specs := LoadRegistryOrEmpty("/nonexistent/path.yaml")
	if specs != nil {
		t.Fatalf("expected nil for missing file, got %d specs", len(specs))
	}
}

func TestFilterByRuntime(t *testing.T) {
	path := writeTestRegistry(t)
	specs, _ := LoadRegistry(path)

	cli := FilterByRuntime(specs, "cli")
	if len(cli) != 2 {
		t.Errorf("cli agents: got %d, want 2", len(cli))
	}

	subagent := FilterByRuntime(specs, "subagent")
	if len(subagent) != 1 {
		t.Errorf("subagent agents: got %d, want 1", len(subagent))
	}
}

func TestLoadRealRegistry(t *testing.T) {
	// Test against the actual fleet-registry.yaml if available.
	realPath := filepath.Join("..", "..", "..", "..", "..", "os", "Clavain", "config", "fleet-registry.yaml")
	if _, err := os.Stat(realPath); err != nil {
		t.Skip("fleet-registry.yaml not found at expected relative path")
	}

	specs, err := LoadRegistry(realPath)
	if err != nil {
		t.Fatalf("LoadRegistry (real): %v", err)
	}
	if len(specs) < 20 {
		t.Errorf("expected 20+ agents in real registry, got %d", len(specs))
	}
}
