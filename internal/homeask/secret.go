package homeask

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// SecretMetadata locates an existing secure interaction; it never carries values.
type SecretMetadata struct {
	Thread  string   `json:"thread"`
	Names   []string `json:"names"`
	Path    string   `json:"path"`
	Project string   `json:"project"`
}

var secretThread = regexp.MustCompile(`^thr_[23456789abcdefghijkmnpqrstuvwxyz]{10}$`)
var secretName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ParseSecret returns a constant error so malformed input is never echoed.
func ParseSecret(body string) (*SecretMetadata, error) {
	bad := errors.New("invalid home-secret metadata")
	var keys map[string]json.RawMessage
	if json.Unmarshal([]byte(body), &keys) != nil || len(keys) != 4 {
		return nil, bad
	}
	for k := range keys {
		if k != "thread" && k != "names" && k != "path" && k != "project" {
			return nil, bad
		}
	}
	d := json.NewDecoder(strings.NewReader(body))
	d.DisallowUnknownFields()
	var m SecretMetadata
	if d.Decode(&m) != nil || d.Decode(new(any)) != io.EOF {
		return nil, bad
	}
	if !secretThread.MatchString(m.Thread) || len(m.Names) == 0 || !strings.HasPrefix(m.Path, "/") || strings.TrimSpace(m.Project) == "" {
		return nil, bad
	}
	for _, s := range []string{m.Path, m.Project} {
		for _, r := range s {
			if r < 32 || r == 127 {
				return nil, bad
			}
		}
	}
	seen := map[string]bool{}
	for _, n := range m.Names {
		if !secretName.MatchString(n) || n == "__proto__" || seen[n] {
			return nil, bad
		}
		seen[n] = true
	}
	return &m, nil
}

func (r CardRequest) secretBlockChecked() (string, error) {
	raw, err := json.Marshal(r.Secret)
	if err != nil {
		return "", errors.New("invalid home-secret metadata")
	}
	m, err := ParseSecret(string(raw))
	if err != nil {
		return "", err
	}
	raw, err = json.Marshal(m)
	if err != nil {
		return "", errors.New("invalid home-secret metadata")
	}
	return "```home-secret\n" + string(raw) + "\n```\n", nil
}
func secretEqual(a *SecretMetadata, b map[string]any) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	raw, _ := json.Marshal(b)
	m, err := ParseSecret(string(raw))
	if err != nil {
		return false
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(m)
	return bytes.Equal(x, y)
}

// CheckSecretDelivery preserves ordinary batching but refuses to queue expiring interactions.
func (f *CardFiler) CheckSecretDelivery(r CardRequest) error {
	if r.Secret == nil {
		return nil
	}
	if err := f.ensureWeekdayHold(); err != nil {
		return err
	}
	return checkSecretDeliveryAt(r, f.clock(), f.WeekdayHoldSetting())
}
func checkSecretDeliveryAt(r CardRequest, at time.Time, setting WeekdayHoldSetting) error {
	if r.Secret == nil {
		return nil
	}
	if r.Secret["thread"] != r.Thread && (r.Owner == "" || r.Secret["thread"] != r.Owner) {
		return fmt.Errorf("%w: --secret thread must match BB_THREAD_ID or --owner-thread", ErrInvalid)
	}
	if r.NonBlocking {
		return fmt.Errorf("%w: --secret cannot be combined with --non-blocking: the secret request expires before the batch releases", ErrInvalid)
	}
	held, err := weeklyHold(at, r, setting)
	if err != nil {
		return err
	}
	if held {
		return fmt.Errorf("%w: --secret cannot use the weekly hold: the secret request expires before the batch releases; use --urgent for immediate delivery", ErrInvalid)
	}
	return nil
}
