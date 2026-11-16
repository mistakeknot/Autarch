package homeask

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBBBinaryFallback(t *testing.T) {
	for _, source := range []string{"configured", "local"} {
		t.Run(source, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local", "bin")
			if err := os.MkdirAll(bin, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(bin, "bb")
			if source == "configured" {
				path = filepath.Join(t.TempDir(), "configured-bb")
			}
			if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '{\"ok\":true}'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			t.Setenv("PATH", t.TempDir())
			t.Setenv("BB_CLI", "")
			if source == "configured" {
				t.Setenv("BB_CLI", path)
			}
			if got, err := ResolveBBBinary(); err != nil || got != path {
				t.Fatalf("expected %s executable, got %q: %v", source, got, err)
			}
			f := &CardFiler{}
			r := f.exec(context.Background(), nil, "status")
			if r.failed() {
				t.Fatalf("bb not found through %s: %+v", source, r)
			}
		})
	}
}

func TestConfiguredBBWinsOverPATH(t *testing.T) {
	dir := t.TempDir()
	configured := filepath.Join(dir, "configured")
	pathBin := filepath.Join(dir, "bb")
	for _, p := range []string{configured, pathBin} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BB_CLI", configured)
	t.Setenv("PATH", dir)
	got, err := ResolveBBBinary()
	if err != nil || got != configured {
		t.Fatal(got, err)
	}
	if err := os.Chmod(configured, 0600); err != nil {
		t.Fatal(err)
	}
	got, err = ResolveBBBinary()
	if err != nil || got != pathBin {
		t.Fatal("non-executable override did not fall back", got, err)
	}
}

func TestUnusableConfiguredBBWarnsOnStderr(t *testing.T) {
	for _, source := range []string{"missing", "non-executable"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			configured := filepath.Join(dir, "configured")
			if source == "non-executable" {
				if err := os.WriteFile(configured, []byte("not executable"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			fallback := filepath.Join(dir, "bb")
			if err := os.WriteFile(fallback, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("BB_CLI", configured)
			t.Setenv("PATH", dir)
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			original := os.Stderr
			os.Stderr = w
			defer func() { os.Stderr = original }()
			got, resolveErr := ResolveBBBinary()
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			warning, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if resolveErr != nil || got != fallback {
				t.Fatalf("fallback = %q, %v", got, resolveErr)
			}
			if !strings.Contains(string(warning), "warning: BB_CLI") || !strings.Contains(string(warning), "unusable") {
				t.Fatalf("missing unusable override warning on stderr: %q", warning)
			}
		})
	}
}
