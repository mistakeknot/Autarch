package mycroft

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPreDispatchConfig(t *testing.T) {
	for _, tc := range []struct {
		name, yaml string
		command    []string
		timeout    time.Duration
	}{
		{"absent", "tier: 0\n", nil, 30 * time.Second},
		{"default_timeout", "pre_dispatch_check:\n  command: [check, 'two words']\n", []string{"check", "two words"}, 30 * time.Second},
		{"explicit_timeout", "pre_dispatch_check:\n  command: [check]\n  timeout: 2s\n", []string{"check"}, 2 * time.Second},
		{"empty", "pre_dispatch_check:\n  command: []\n", []string{}, 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.PreDispatchCheck.Command, tc.command) || time.Duration(cfg.PreDispatchCheck.Timeout) != tc.timeout {
				t.Fatalf("check=%+v want command=%v timeout=%s", cfg.PreDispatchCheck, tc.command, tc.timeout)
			}
		})
	}
	for _, value := range []string{"command: check", "command: [check]\n  timeout: broken"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("pre_dispatch_check:\n  "+value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("accepted invalid config %q", value)
		}
	}
}
