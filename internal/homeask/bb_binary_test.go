package homeask

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBBBinaryFallback(t *testing.T) {
	for _, source := range []string{"configured", "local"} {
		t.Run(source, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local", "bin")
			os.MkdirAll(bin, 0700)
			path := filepath.Join(bin, "bb")
			os.WriteFile(path, []byte("#!/bin/sh\nprintf '{\"ok\":true}'\n"), 0700)
			t.Setenv("HOME", home)
			t.Setenv("PATH", t.TempDir())
			t.Setenv("BB_CLI", "")
			if source == "configured" {
				t.Setenv("BB_CLI", path)
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
		os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0700)
	}
	t.Setenv("BB_CLI", configured)
	t.Setenv("PATH", dir)
	got, err := ResolveBBBinary()
	if err != nil || got != configured {
		t.Fatal(got, err)
	}
	os.Chmod(configured, 0600)
	got, err = ResolveBBBinary()
	if err != nil || got != pathBin {
		t.Fatal("non-executable override did not fall back", got, err)
	}
}
