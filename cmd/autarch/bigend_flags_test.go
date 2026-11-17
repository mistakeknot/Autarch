package main

import (
	"fmt"
	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/pkg/netguard"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBigendFlagOverrides(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		host string
		port int
		root string
	}{
		{"absent", nil, "127.0.0.2", 9000, "configured"},
		{"explicit", []string{"--host=127.0.0.3", "--port=8101", "--scan-root=chosen"}, "127.0.0.3", 8101, "chosen"},
		{"explicit defaults", []string{"--host=127.0.0.1", "--port=8099"}, "127.0.0.1", 8099, "configured"},
		{"config nonloopback", nil, "0.0.0.0", 9000, "configured"},
		{"flag nonloopback", []string{"--host=0.0.0.0"}, "0.0.0.0", 9000, "configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := "127.0.0.2"
			if tc.name == "config nonloopback" {
				host = "0.0.0.0"
			}
			p := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(p, []byte(fmt.Sprintf("[server]\nhost=%q\nport=9000\n[discovery]\nscan_roots=['configured']\n", host)), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(p)
			if err != nil {
				t.Fatal(err)
			}
			cmd := bigendCmd()
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			if err := applyBigendFlagOverrides(cmd, cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Server.Host != tc.host || cfg.Server.Port != tc.port || len(cfg.Discovery.ScanRoots) != 1 || cfg.Discovery.ScanRoots[0] != tc.root {
				t.Fatalf("got host=%q port=%d roots=%q", cfg.Server.Host, cfg.Server.Port, cfg.Discovery.ScanRoots)
			}
			if tc.host == "0.0.0.0" {
				// Validate the resolved address without ever opening a listener.
				err := netguard.EnsureLocalOnly(fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port))
				if err == nil || !strings.Contains(err.Error(), "refusing to bind non-loopback") {
					t.Fatalf("expected bind refusal, got %v", err)
				}
			}
		})
	}
}

func TestBigendEmptyScanRoot(t *testing.T) {
	for _, root := range []string{"", " ", "\t\n"} {
		t.Run(fmt.Sprintf("%q", root), func(t *testing.T) {
			cfg, err := config.Load(filepath.Join(t.TempDir(), "absent.toml"))
			if err != nil {
				t.Fatal(err)
			}
			cmd := bigendCmd()
			if err := cmd.ParseFlags([]string{"--scan-root=" + root}); err != nil {
				t.Fatal(err)
			}
			err = applyBigendFlagOverrides(cmd, cfg)
			if err == nil || !strings.Contains(err.Error(), "--scan-root") {
				t.Fatalf("expected --scan-root error, got %v", err)
			}
		})
	}
}
