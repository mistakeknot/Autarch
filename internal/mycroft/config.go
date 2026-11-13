package mycroft

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config maps .autarch/mycroft/config.yaml.
type Config struct {
	PreDispatchCheck PreDispatchCheck `yaml:"pre_dispatch_check,omitempty"`
}

// PreDispatchCheck optionally checks a proposed assignment with a direct argv command.
type PreDispatchCheck struct {
	Command []string `yaml:"command"`
	Timeout Duration `yaml:"timeout"`
}

// Duration wraps time.Duration for YAML serialization (e.g., "24h").
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{PreDispatchCheck: PreDispatchCheck{Timeout: Duration(30 * time.Second)}}
}

// LoadConfig reads a config file, falling back to defaults for missing fields.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // missing file → defaults
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}

// SaveConfig writes the config to disk.
func SaveConfig(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
