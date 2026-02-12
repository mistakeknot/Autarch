package agenttargets

import (
	"os/exec"
	"strings"
	"testing"
)

func TestApplyEnv_AlwaysSetsAutarchEnabled(t *testing.T) {
	cmd := exec.Command("echo")
	cfg := DispatchConfig{} // no extra env
	applyEnv(cmd, cfg)

	found := false
	for _, e := range cmd.Env {
		if e == "AUTARCH_ENABLED=1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AUTARCH_ENABLED=1 not found in cmd.Env")
	}
}

func TestApplyEnv_InjectsCustomVars(t *testing.T) {
	cmd := exec.Command("echo")
	cfg := DispatchConfig{
		Env: map[string]string{
			"AUTARCH_TOOL":    "gurgeh",
			"AUTARCH_SPEC_ID": "PRD-001",
		},
	}
	applyEnv(cmd, cfg)

	envMap := make(map[string]string)
	for _, e := range cmd.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if envMap["AUTARCH_ENABLED"] != "1" {
		t.Error("AUTARCH_ENABLED not set")
	}
	if envMap["AUTARCH_TOOL"] != "gurgeh" {
		t.Errorf("AUTARCH_TOOL = %q, want %q", envMap["AUTARCH_TOOL"], "gurgeh")
	}
	if envMap["AUTARCH_SPEC_ID"] != "PRD-001" {
		t.Errorf("AUTARCH_SPEC_ID = %q, want %q", envMap["AUTARCH_SPEC_ID"], "PRD-001")
	}
}

func TestApplyEnv_InheritsParentEnvironment(t *testing.T) {
	cmd := exec.Command("echo")
	cfg := DispatchConfig{
		Env: map[string]string{"CUSTOM": "value"},
	}
	applyEnv(cmd, cfg)

	// cmd.Env should contain parent environment (HOME, PATH, etc.)
	hasPath := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "PATH=") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Error("parent PATH not inherited in cmd.Env")
	}
}
