package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server    ServerConfig    `toml:"server"`
	Discovery DiscoveryConfig `toml:"discovery"`
	Tmux      TmuxConfig      `toml:"tmux"`
}

type ServerConfig struct {
	Port int    `toml:"port"`
	Host string `toml:"host"`
}

type DiscoveryConfig struct {
	ScanRoots       []string      `toml:"scan_roots"`
	ScanInterval    time.Duration `toml:"scan_interval"`
	ExcludePatterns []string      `toml:"exclude_patterns"`
}

type TmuxConfig struct {
	SocketPath string `toml:"socket_path"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: 8099,
			Host: "0.0.0.0",
		},
		Discovery: DiscoveryConfig{
			ScanRoots:       []string{expandHome("~/projects")},
			ScanInterval:    30 * time.Second,
			ExcludePatterns: []string{"node_modules", ".git", "vendor", "target"},
		},
	}

	// Try default paths if not specified
	if path == "" {
		candidates := []string{
			expandHome("~/.config/vauxhall/config.toml"),
			"./config.toml",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}

	// Load from file if exists
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, cfg); err != nil {
				return nil, err
			}
		}
	}

	// Expand home directories
	for i, root := range cfg.Discovery.ScanRoots {
		cfg.Discovery.ScanRoots[i] = expandHome(root)
	}

	return cfg, nil
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
