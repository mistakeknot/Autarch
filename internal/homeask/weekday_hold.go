package homeask

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// EnvCardWeekdayHold is the supported switch for the Monday–Thursday-noon
// ordinary-card hold. See ParseWeekdayHoldValue for the accepted values.
const EnvCardWeekdayHold = "AUTARCH_CARD_WEEKDAY_HOLD"

const (
	weekdayHoldOn weekdayHoldMode = iota + 1
	weekdayHoldOff
	weekdayHoldOffList
)

type weekdayHoldMode int

// WeekdayHoldSetting is a parsed AUTARCH_CARD_WEEKDAY_HOLD value.
// A zero value is not loaded and must not be used with weeklyHold.
type WeekdayHoldSetting struct {
	loaded     bool
	mode       weekdayHoldMode
	tokenSet   map[string]struct{}
	ConfigPath string
	Source     string // "env", "file", or "default"
}

func (s WeekdayHoldSetting) Loaded() bool       { return s.loaded }
func (s WeekdayHoldSetting) ConfigFile() string { return s.ConfigPath }

// weekdayHoldState is heap-allocated and shared by CardFiler value copies.
// sync.Once must not be a CardFiler field: release and readback copy the filer.
type weekdayHoldState struct {
	once     sync.Once
	setting  WeekdayHoldSetting
	err      error
	prefixes map[string]string
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

// ParseWeekdayHoldValue parses one setting value.
// Empty is treated as unset (the caller should fall through). on / off are
// case-insensitive. off:<token>[,<token>] disables the hold only for those
// Tasks project ids, names, or prefixes (matched case-insensitively).
func ParseWeekdayHoldValue(raw string) (WeekdayHoldSetting, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return WeekdayHoldSetting{}, nil
	}
	low := strings.ToLower(v)
	switch {
	case low == "on":
		return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOn, Source: "value"}, nil
	case low == "off":
		return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOff, Source: "value"}, nil
	case strings.HasPrefix(low, "off:"):
		rest := strings.TrimSpace(v[len("off:"):])
		if rest == "" {
			return WeekdayHoldSetting{}, fmt.Errorf("%w: %s %q has an empty project list", ErrInvalid, EnvCardWeekdayHold, raw)
		}
		tokens := map[string]struct{}{}
		for _, part := range strings.Split(rest, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				return WeekdayHoldSetting{}, fmt.Errorf("%w: %s %q has an empty project token", ErrInvalid, EnvCardWeekdayHold, raw)
			}
			tokens[strings.ToLower(part)] = struct{}{}
		}
		return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOffList, tokenSet: tokens, Source: "value"}, nil
	default:
		return WeekdayHoldSetting{}, fmt.Errorf("%w: %s %q is not on, off, or off:<project>[,<project>]", ErrInvalid, EnvCardWeekdayHold, raw)
	}
}

// LoadWeekdayHoldSetting reads the env (first wins if non-empty), then
// <configDir>/autarch/needsmk.env, then defaults to on.
// configDir is os.UserConfigDir when empty.
func LoadWeekdayHoldSetting(configDir string) (WeekdayHoldSetting, error) {
	path, err := weekdayHoldConfigPath(configDir)
	if err != nil {
		return WeekdayHoldSetting{}, err
	}
	if s, err := ParseWeekdayHoldValue(os.Getenv(EnvCardWeekdayHold)); err != nil {
		s.ConfigPath = path
		return WeekdayHoldSetting{}, err
	} else if s.loaded {
		s.ConfigPath = path
		s.Source = "env"
		return s, nil
	}
	if raw, ok, err := readWeekdayHoldFile(path); err != nil {
		return WeekdayHoldSetting{}, err
	} else if ok {
		s, err := ParseWeekdayHoldValue(raw)
		if err != nil {
			return WeekdayHoldSetting{}, err
		}
		if !s.loaded {
			s = WeekdayHoldSetting{loaded: true, mode: weekdayHoldOn, Source: "default", ConfigPath: path}
			return s, nil
		}
		s.ConfigPath = path
		s.Source = "file"
		return s, nil
	}
	return WeekdayHoldSetting{loaded: true, mode: weekdayHoldOn, Source: "default", ConfigPath: path}, nil
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

func readWeekdayHoldFile(path string) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("%w: reading %s: %v", ErrInvalid, path, err)
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
			return "", false, fmt.Errorf("%w: %s:%d is not KEY=VALUE", ErrInvalid, path, lineNo)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != EnvCardWeekdayHold {
			continue
		}
		if found {
			return "", false, fmt.Errorf("%w: %s repeats %s", ErrInvalid, path, EnvCardWeekdayHold)
		}
		found = true
		value = v
	}
	if err := sc.Err(); err != nil {
		return "", false, fmt.Errorf("%w: reading %s: %v", ErrInvalid, path, err)
	}
	return value, found, nil
}

func (s WeekdayHoldSetting) disables(project, resolvedPrefix string) bool {
	switch s.mode {
	case weekdayHoldOff:
		return true
	case weekdayHoldOffList:
		return s.matches(project) || s.matches(resolvedPrefix)
	default:
		return false
	}
}

func (s WeekdayHoldSetting) matches(token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	_, ok := s.tokenSet[token]
	return ok
}

func weeklyHold(at time.Time, r CardRequest, setting WeekdayHoldSetting, resolvedPrefix string) (bool, error) {
	if !setting.loaded {
		return false, fmt.Errorf("%w: weekday hold setting not loaded", ErrInvalid)
	}
	// mk-bzi8s.6: merge and root-run asks are operational, like urgent and
	// keep-list asks, and bypass the weekday hold using the pilot definition.
	if !ordinary(r) {
		return false, nil
	}
	if setting.disables(r.Project, resolvedPrefix) {
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

func (f *CardFiler) setWeekdayPrefixMap(m map[string]string) {
	f.weekdayState().prefixes = m
}

// tasksPrefixMap maps a Tasks project id, name, or prefix (lowercased for
// name/prefix) to the canonical prefix. A failed list returns (nil, err).
func (f *CardFiler) tasksPrefixMap(ctx context.Context) (map[string]string, error) {
	var projects struct {
		Projects []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Prefix string `json:"prefix"`
		} `json:"projects"`
	}
	if err := f.call(ctx, "", &projects, "tasks", "project", "list", "--json"); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, p := range projects.Projects {
		canon := p.Prefix
		if canon == "" {
			canon = p.Name
		}
		if canon == "" {
			canon = p.ID
		}
		if canon == "" {
			continue
		}
		if p.ID != "" {
			out[p.ID] = canon
			out[strings.ToLower(p.ID)] = canon
		}
		if p.Name != "" {
			out[strings.ToLower(p.Name)] = canon
		}
		if p.Prefix != "" {
			out[strings.ToLower(p.Prefix)] = canon
		}
	}
	return out, nil
}

func prefixFromMap(project string, m map[string]string) string {
	project = strings.TrimSpace(project)
	if project == "" || m == nil {
		return ""
	}
	if v, ok := m[project]; ok {
		return v
	}
	if v, ok := m[strings.ToLower(project)]; ok {
		return v
	}
	return ""
}

func (f *CardFiler) resolvedPrefix(ctx context.Context, project string) string {
	st := f.weekdayState()
	if st.prefixes != nil {
		return prefixFromMap(project, st.prefixes)
	}
	m, err := f.tasksPrefixMap(ctx)
	if err != nil {
		return ""
	}
	st.prefixes = m
	return prefixFromMap(project, m)
}

func (f *CardFiler) holdPrefix(ctx context.Context, r CardRequest) string {
	if err := f.ensureWeekdayHold(); err != nil {
		return ""
	}
	if f.WeekdayHoldSetting().mode != weekdayHoldOffList {
		return strings.TrimSpace(r.Project)
	}
	return f.resolvedPrefix(ctx, r.Project)
}

func (s WeekdayHoldSetting) Value() string {
	switch s.mode {
	case weekdayHoldOn:
		return "on"
	case weekdayHoldOff:
		return "off"
	case weekdayHoldOffList:
		keys := make([]string, 0, len(s.tokenSet))
		for k := range s.tokenSet {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "off:" + strings.Join(keys, ",")
	default:
		return ""
	}
}

// CheckWeekdayHoldConfig validates a needsmk.env file. The AUTARCH_CARD_WEEKDAY_HOLD
// key must be present and parseable; an empty or missing key is invalid.
func CheckWeekdayHoldConfig(path string) (WeekdayHoldSetting, error) {
	raw, ok, err := readWeekdayHoldFile(path)
	if err != nil {
		return WeekdayHoldSetting{}, err
	}
	if !ok {
		return WeekdayHoldSetting{}, fmt.Errorf("%w: %s does not set %s", ErrInvalid, path, EnvCardWeekdayHold)
	}
	s, err := ParseWeekdayHoldValue(raw)
	if err != nil {
		return WeekdayHoldSetting{}, err
	}
	if !s.loaded {
		return WeekdayHoldSetting{}, fmt.Errorf("%w: %s has an empty %s", ErrInvalid, path, EnvCardWeekdayHold)
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
	held, err = weeklyHold(at, r, setting, f.holdPrefix(ctx, r))
	if err != nil {
		return false, "", setting.ConfigPath, err
	}
	if held {
		reason = "weekday-window"
	}
	return held, reason, setting.ConfigPath, nil
}
