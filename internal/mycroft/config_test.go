package mycroft

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	if got := time.Duration(DefaultConfig().PreDispatchCheck.Timeout); got != 30*time.Second {
		t.Fatalf("timeout=%s", got)
	}
}
func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil || !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}
func TestLoadConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{PreDispatchCheck: PreDispatchCheck{Command: []string{"check", "two words"}, Timeout: Duration(time.Second)}}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
