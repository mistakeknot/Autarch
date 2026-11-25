package homeask

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "homeask-config-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", root)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	os.Setenv("AppData", filepath.Join(root, "config"))
	os.Setenv(EnvCardWeekdayHold, "")
	_ = os.MkdirAll(filepath.Join(root, "config"), 0700)
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
