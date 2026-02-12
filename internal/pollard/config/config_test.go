package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigSaveLoad_Cases(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "round trip default config",
			cfg:  DefaultConfig(),
		},
		{
			name: "empty config",
			cfg:  &Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectPath := t.TempDir()

			if err := tt.cfg.Save(projectPath); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			got, err := Load(projectPath)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			want := cloneConfig(t, tt.cfg)
			want.applyDefaults()

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Save/Load mismatch:\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestConfigLoad_MissingFileReturnsDefault(t *testing.T) {
	projectPath := t.TempDir()

	got, err := Load(projectPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := DefaultConfig()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing-file load mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestConfigLoad_StrictRejectsUnknownField(t *testing.T) {
	projectPath := t.TempDir()
	pollardDir := filepath.Join(projectPath, ".pollard")
	if err := os.MkdirAll(pollardDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	configYAML := "hunters: {}\nunknown_field: true\n"
	if err := os.WriteFile(filepath.Join(pollardDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(projectPath)
	if err == nil {
		t.Fatal("Load() error = nil, want strict decode error")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("Load() error = %v, want unknown_field mention", err)
	}
}

func TestConfigLoad_MinimalYAMLAppliesDefaults(t *testing.T) {
	projectPath := t.TempDir()
	pollardDir := filepath.Join(projectPath, ".pollard")
	if err := os.MkdirAll(pollardDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	configYAML := "hunters:\n  github-scout:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(pollardDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := Load(projectPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	hunter, ok := got.Hunters["github-scout"]
	if !ok {
		t.Fatal("github-scout missing from loaded config")
	}

	if got.Speed != "slow" {
		t.Fatalf("Speed = %q, want %q", got.Speed, "slow")
	}
	if got.Defaults.MaxResults != 50 || got.Defaults.Interval != "6h" {
		t.Fatalf("Defaults = %#v, want max_results=50 interval=6h", got.Defaults)
	}
	if got.Linking.Mode != "suggest" || got.Linking.ConfidenceThreshold != 0.8 {
		t.Fatalf("Linking = %#v, want suggest/0.8", got.Linking)
	}
	if hunter.MaxResults != 50 || hunter.Interval != "6h" {
		t.Fatalf("Hunter defaults = %#v, want max_results=50 interval=6h", hunter)
	}
	if got.Pipeline.Synthesizer.Parallelism != 3 || got.Pipeline.Synthesizer.Timeout != "2m" {
		t.Fatalf("Synthesizer defaults = %#v, want parallelism=3 timeout=2m", got.Pipeline.Synthesizer)
	}
	if got.Pipeline.Modes.Balanced.FetchDepth != "standard" || !got.Pipeline.Modes.Balanced.Synthesize || got.Pipeline.Modes.Balanced.SynthesizeLimit != 10 {
		t.Fatalf("Balanced mode defaults = %#v, want standard/true/10", got.Pipeline.Modes.Balanced)
	}
	if got.Scoring.Weights.Engagement != 0.25 || got.Scoring.HalfLives.Trends != "168h" || got.Scoring.Thresholds.High != 0.7 {
		t.Fatalf("Scoring defaults = %#v, want engagement=0.25 trends=168h high=0.7", got.Scoring)
	}
}

func cloneConfig(t *testing.T, in *Config) *Config {
	t.Helper()

	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	var out Config
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	return &out
}
