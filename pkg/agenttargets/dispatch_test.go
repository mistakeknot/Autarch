package agenttargets

import (
	"context"
	"testing"
	"time"
)

func TestDefaultDispatchConfig(t *testing.T) {
	cfg := DefaultDispatchConfig()

	if cfg.PreferredBackend != BackendSubscriptionCLI {
		t.Errorf("preferred backend = %q, want %q", cfg.PreferredBackend, BackendSubscriptionCLI)
	}
	if cfg.PreferredAgent != "" {
		t.Errorf("preferred agent = %q, want empty (auto-detect)", cfg.PreferredAgent)
	}
	if cfg.Timeout != 30*time.Minute {
		t.Errorf("timeout = %v, want 30m", cfg.Timeout)
	}
	if !cfg.Verbose {
		t.Error("verbose should be true by default")
	}
	if !cfg.Print {
		t.Error("print should be true by default")
	}
}

func TestBackendTypeConstants(t *testing.T) {
	// Verify the string values are what we expect for TOML serialization.
	if BackendSubscriptionCLI != "subscription-cli" {
		t.Errorf("BackendSubscriptionCLI = %q", BackendSubscriptionCLI)
	}
	if BackendAPI != "api" {
		t.Errorf("BackendAPI = %q", BackendAPI)
	}
}

func TestStreamEventTypes(t *testing.T) {
	// Verify enum ordering is stable (important for any future serialization).
	if StreamText != 0 {
		t.Errorf("StreamText = %d, want 0", StreamText)
	}
	if StreamResult != 5 {
		t.Errorf("StreamResult = %d, want 5", StreamResult)
	}
	if StreamSessionID != 7 {
		t.Errorf("StreamSessionID = %d, want 7", StreamSessionID)
	}
}

func TestDetectPreferredTool(t *testing.T) {
	// This test runs on real environment — just verify it returns something valid.
	tool, found := DetectPreferredTool(context.Background())
	if found {
		switch tool.Name {
		case "claude", "codex":
			// Valid
			if tool.Path == "" {
				t.Error("detected tool has empty path")
			}
		default:
			t.Errorf("unexpected tool %q", tool.Name)
		}
	}
	// Not found is also valid — CI may not have agents installed.
}
