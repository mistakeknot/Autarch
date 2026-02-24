package agenttargets

import (
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type legacyAgent struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

type fileDispatchConfig struct {
	PreferredBackend string `toml:"preferred_backend"`
	PreferredAgent   string `toml:"preferred_agent"`
	Timeout          string `toml:"timeout"`
}

type fileConfig struct {
	Targets  map[string]Target      `toml:"targets"`
	Agents   map[string]legacyAgent `toml:"agents"`
	Dispatch fileDispatchConfig     `toml:"dispatch"`
}

func Load(globalPath, projectRoot string) (Registry, Registry, error) {
	global, err := loadRegistry(globalPath)
	if err != nil {
		return Registry{}, Registry{}, err
	}
	project, err := loadProjectRegistry(projectRoot)
	if err != nil {
		return Registry{}, Registry{}, err
	}
	return global, project, nil
}

func loadRegistry(path string) (Registry, error) {
	if path == "" {
		return Registry{Targets: map[string]Target{}}, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Registry{Targets: map[string]Target{}}, nil
		}
		return Registry{}, err
	}
	var cfg fileConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Registry{}, err
	}
	return registryFromConfig(cfg), nil
}

func loadProjectRegistry(projectRoot string) (Registry, error) {
	if projectRoot == "" {
		return Registry{Targets: map[string]Target{}}, nil
	}
	// Check .gurgeh first, then .praude for backward compatibility
	gurgDir := filepath.Join(projectRoot, ".gurgeh")
	praudeDir := filepath.Join(projectRoot, ".praude")
	configDir := gurgDir
	if _, err := os.Stat(gurgDir); os.IsNotExist(err) {
		if _, err := os.Stat(praudeDir); err == nil {
			configDir = praudeDir
		}
	}

	agentsPath := filepath.Join(configDir, "agents.toml")
	if _, err := os.Stat(agentsPath); err == nil {
		return loadRegistry(agentsPath)
	} else if err != nil && !os.IsNotExist(err) {
		return Registry{}, err
	}

	compatPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(compatPath); err != nil {
		if os.IsNotExist(err) {
			return Registry{Targets: map[string]Target{}}, nil
		}
		return Registry{}, err
	}
	var cfg fileConfig
	if _, err := toml.DecodeFile(compatPath, &cfg); err != nil {
		return Registry{}, err
	}
	return registryFromConfig(cfg), nil
}

func registryFromConfig(cfg fileConfig) Registry {
	reg := Registry{Targets: map[string]Target{}}
	for name, target := range cfg.Targets {
		if target.Name == "" {
			target.Name = name
		}
		reg.Targets[name] = target
	}
	for name, agent := range cfg.Agents {
		reg.Targets[name] = Target{
			Name:    name,
			Type:    TargetCommand,
			Command: agent.Command,
			Args:    agent.Args,
		}
	}
	return reg
}

// LoadDispatchConfig reads project-level dispatch preferences from agents.toml
// and applies environment variable overrides. Returns DefaultDispatchConfig() if
// no config file is found.
func LoadDispatchConfig(projectRoot string) DispatchConfig {
	cfg := DefaultDispatchConfig()

	if projectRoot == "" {
		applyDispatchEnvOverrides(&cfg)
		return cfg
	}

	// Look for agents.toml in .gurgeh/ or .praude/
	gurgDir := filepath.Join(projectRoot, ".gurgeh")
	praudeDir := filepath.Join(projectRoot, ".praude")
	configDir := gurgDir
	if _, err := os.Stat(gurgDir); os.IsNotExist(err) {
		if _, err := os.Stat(praudeDir); err == nil {
			configDir = praudeDir
		}
	}

	agentsPath := filepath.Join(configDir, "agents.toml")
	if _, err := os.Stat(agentsPath); err != nil {
		applyDispatchEnvOverrides(&cfg)
		return cfg
	}

	var fileCfg fileConfig
	if _, err := toml.DecodeFile(agentsPath, &fileCfg); err != nil {
		applyDispatchEnvOverrides(&cfg)
		return cfg
	}

	d := fileCfg.Dispatch
	if d.PreferredBackend != "" {
		cfg.PreferredBackend = BackendType(d.PreferredBackend)
	}
	if d.PreferredAgent != "" {
		cfg.PreferredAgent = d.PreferredAgent
	}
	if d.Timeout != "" {
		if dur, err := time.ParseDuration(d.Timeout); err == nil {
			cfg.Timeout = dur
		}
	}

	applyDispatchEnvOverrides(&cfg)
	return cfg
}

// applyDispatchEnvOverrides applies environment variable overrides to a DispatchConfig.
func applyDispatchEnvOverrides(cfg *DispatchConfig) {
	if v := os.Getenv("AUTARCH_DISPATCH_BACKEND"); v != "" {
		cfg.PreferredBackend = BackendType(v)
	}
	if v := os.Getenv("AUTARCH_DISPATCH_AGENT"); v != "" {
		cfg.PreferredAgent = v
	}
}
