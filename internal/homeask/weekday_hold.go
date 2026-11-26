package homeask

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// EnvCardWeekdayHold is the supported switch for the Monday–Thursday-noon
// ordinary-card hold. Accepted values are on and off.
const EnvCardWeekdayHold = "AUTARCH_CARD_WEEKDAY_HOLD"

const (
	weekdayHoldOn weekdayHoldMode = iota + 1
	weekdayHoldOff
)

type weekdayHoldMode int

// WeekdayHoldSetting is a parsed AUTARCH_CARD_WEEKDAY_HOLD value.
// A zero value is not loaded and must not be used with weeklyHold.
type WeekdayHoldSetting struct {
	loaded     bool
	mode       weekdayHoldMode
	ConfigPath string
	Source     string // "env", "file", or "default"
}

func (s WeekdayHoldSetting) Loaded() bool       { return s.loaded }
func (s WeekdayHoldSetting) ConfigFile() string { return s.ConfigPath }

// weekdayHoldState is heap-allocated and shared by CardFiler value copies.
// sync.Once must not be a CardFiler field: release and readback copy the filer.
type weekdayHoldState struct {
	once    sync.Once
	setting WeekdayHoldSetting
	err     error
}

func (f *CardFiler) weekdayState() *weekdayHoldState {
	if p := atomic.LoadPointer(&f.weekdayHoldPtr); p != nil {
		return (*weekdayHoldState)(p)
	}
	fresh := &weekdayHoldState{}
	if atomic.CompareAndSwapPointer(&f.weekdayHoldPtr, nil, unsafe.Pointer(fresh)) {
		return fresh
	}
	return (*weekdayHoldState)(atomic.LoadPointer(&f.weekdayHoldPtr))
}

// validateWeekdayHoldValue is the one rule for a present value, including an
// empty assignment. The process environment treats empty as unset before calling this.
func validateWeekdayHoldValue(raw string) (WeekdayHoldSetting, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return WeekdayHoldSetting{}, fmt.Errorf("%w: %s is empty", ErrInvalid, EnvCardWeekdayHold)
	}
	switch strings.ToLower(v) {
	case "on":
		return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOn}, nil
	case "off":
		return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOff}, nil
	default:
		return WeekdayHoldSetting{}, fmt.Errorf("%w: %s %q is not on or off", ErrInvalid, EnvCardWeekdayHold, raw)
	}
}

// LoadWeekdayHoldSetting reads the env (a non-empty value wins), then
// <configDir>/autarch/needsmk.env, then defaults to on only when that file is absent.
// configDir is os.UserConfigDir when empty. The env is parsed before the config dir is required.
func LoadWeekdayHoldSetting(configDir string) (WeekdayHoldSetting, error) {
	if raw := strings.TrimSpace(os.Getenv(EnvCardWeekdayHold)); raw != "" {
		s, err := validateWeekdayHoldValue(raw)
		if err != nil {
			return WeekdayHoldSetting{}, err
		}
		s.Source = "env"
		if path, pathErr := weekdayHoldConfigPath(configDir); pathErr == nil {
			s.ConfigPath = path
		}
		return s, nil
	}
	path, err := weekdayHoldConfigPath(configDir)
	if err != nil {
		return WeekdayHoldSetting{}, err
	}
	raw, err := readWeekdayHoldFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOn, Source: "default", ConfigPath: path}, nil
		}
		return WeekdayHoldSetting{}, err
	}
	s, err := validateWeekdayHoldValue(raw)
	if err != nil {
		return WeekdayHoldSetting{}, fmt.Errorf("%s: %w", path, err)
	}
	s.ConfigPath = path
	s.Source = "file"
	return s, nil
}

func weekdayHoldConfigPath(configDir string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("%w: user config dir: %v", ErrInvalid, err)
		}
		configDir = dir
	}
	return filepath.Join(configDir, "autarch", "needsmk.env"), nil
}

// readWeekdayHoldFile returns the present AUTARCH_CARD_WEEKDAY_HOLD value.
// A missing file is os.ErrNotExist. Any other key is an error naming path and line.
// A present empty value is returned as "" so validateWeekdayHoldValue can reject it.
func readWeekdayHoldFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	found := false
	value := ""
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return "", fmt.Errorf("%w: %s:%d is not KEY=VALUE", ErrInvalid, path, lineNo)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != EnvCardWeekdayHold {
			return "", fmt.Errorf("%w: %s:%d unknown key %q", ErrInvalid, path, lineNo, k)
		}
		if found {
			return "", fmt.Errorf("%w: %s repeats %s", ErrInvalid, path, EnvCardWeekdayHold)
		}
		found = true
		value = v
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("%w: reading %s: %v", ErrInvalid, path, err)
	}
	if !found {
		return "", fmt.Errorf("%w: %s does not set %s", ErrInvalid, path, EnvCardWeekdayHold)
	}
	return value, nil
}

func weeklyHold(at time.Time, r CardRequest, setting WeekdayHoldSetting) (bool, error) {
	if !setting.loaded {
		return false, fmt.Errorf("%w: weekday hold setting not loaded", ErrInvalid)
	}
	// mk-bzi8s.6: merge and root-run asks are operational, like urgent and
	// keep-list asks, and bypass the weekday hold using the pilot definition.
	if !ordinary(r) {
		return false, nil
	}
	if setting.mode == weekdayHoldOff {
		return false, nil
	}
	local := at.In(pacific)
	day := local.Weekday()
	return day >= time.Monday && day < time.Thursday || day == time.Thursday && local.Hour() < 12, nil
}

func (f *CardFiler) configDir() string {
	return strings.TrimSpace(f.ConfigDir)
}

func (f *CardFiler) LoadWeekdayHold() error {
	st := f.weekdayState()
	s, err := LoadWeekdayHoldSetting(f.configDir())
	if err != nil {
		st.setting = WeekdayHoldSetting{}
		st.err = err
		return err
	}
	st.setting = s
	st.err = nil
	return nil
}

func (f *CardFiler) ensureWeekdayHold() error {
	st := f.weekdayState()
	st.once.Do(func() {
		if st.setting.loaded || st.err != nil {
			return
		}
		_ = f.LoadWeekdayHold()
	})
	if st.err != nil {
		return st.err
	}
	if !st.setting.loaded {
		return fmt.Errorf("%w: weekday hold setting not loaded", ErrInvalid)
	}
	return nil
}

func (f *CardFiler) WeekdayHoldSetting() WeekdayHoldSetting {
	if p := atomic.LoadPointer(&f.weekdayHoldPtr); p != nil {
		return (*weekdayHoldState)(p).setting
	}
	return WeekdayHoldSetting{}
}

func (s WeekdayHoldSetting) Value() string {
	switch s.mode {
	case weekdayHoldOn:
		return "on"
	case weekdayHoldOff:
		return "off"
	default:
		return ""
	}
}

// CheckWeekdayHoldConfig validates a needsmk.env file. The AUTARCH_CARD_WEEKDAY_HOLD
// key must be present and parseable; an empty or missing key is invalid.
func CheckWeekdayHoldConfig(path string) (WeekdayHoldSetting, error) {
	raw, err := readWeekdayHoldFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WeekdayHoldSetting{}, fmt.Errorf("%w: %s does not set %s", ErrInvalid, path, EnvCardWeekdayHold)
		}
		return WeekdayHoldSetting{}, err
	}
	s, err := validateWeekdayHoldValue(raw)
	if err != nil {
		return WeekdayHoldSetting{}, fmt.Errorf("%s: %w", path, err)
	}
	s.Source = "file"
	s.ConfigPath = path
	return s, nil
}

// WeekdayHoldDecision reports whether r would be weekday-held at at (or now if at is zero).
func (f *CardFiler) WeekdayHoldDecision(ctx context.Context, r CardRequest, at time.Time) (held bool, reason, configPath string, err error) {
	if err = f.ensureWeekdayHold(); err != nil {
		return false, "", "", err
	}
	if at.IsZero() {
		at = f.clock()
	}
	setting := f.WeekdayHoldSetting()
	held, err = weeklyHold(at, r, setting)
	if err != nil {
		return false, "", setting.ConfigPath, err
	}
	if held {
		reason = "weekday-window"
	}
	return held, reason, setting.ConfigPath, nil
}
