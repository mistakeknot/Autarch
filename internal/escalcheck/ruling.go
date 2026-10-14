package escalcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxLedger     = 64 << 20
	maxLedgerLine = 1 << 20
)

// ledgerPath is the flag, else $XDG_STATE_HOME/vizier/rulings.jsonl, else
// $HOME/.local/state/vizier/rulings.jsonl.
func ledgerPath(o Options, environ []string) string {
	if o.Rulings != "" {
		return o.Rulings
	}
	base := getenv(environ, "XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(getenv(environ, "HOME"), ".local", "state")
	}
	return filepath.Join(base, "vizier", "rulings.jsonl")
}

// parseObject decodes one JSONL line into its top-level members, rejecting
// duplicate keys, anything that is not an object, and trailing input.
func parseObject(line []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not an object")
	}
	out := map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := kt.(string)
		if !ok {
			return nil, errors.New("bad key")
		}
		if _, dup := out[key]; dup {
			return nil, errors.New("duplicate key " + key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		out[key] = raw
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data after the object")
	}
	return out, nil
}

// rulingID is the row's ID: "id" on the older shape, "ruling_id" on the newer
// revision rows. A row with neither, a non-string, or two different values is
// malformed.
func rulingID(obj map[string]json.RawMessage) (string, bool) {
	a, aok := str(obj["id"])
	b, bok := str(obj["ruling_id"])
	switch {
	case aok && bok:
		return a, a == b
	case aok:
		return a, true
	case bok:
		return b, true
	}
	return "", false
}

func str(raw json.RawMessage) (string, bool) {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

func checkRuling(o Options, env Env) []Item {
	path := ledgerPath(o, env.Environ())
	b, _, err := readRegular(path, maxLedger)
	if err != nil {
		return []Item{
			item("ruling.present", Unknown, "cannot read the ledger: %v", err),
			item("ruling.shape", Unknown, "cannot read the ledger"),
		}
	}
	var matches []map[string]json.RawMessage
	var matchRaw [][]byte
	var matchLine []int
	lineNo := 0
	malformed := 0
	for _, line := range bytes.Split(b, []byte("\n")) {
		lineNo++
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if len(line) > maxLedgerLine {
			malformed++
			continue
		}
		obj, err := parseObject(line)
		if err != nil {
			malformed++
			continue
		}
		id, ok := rulingID(obj)
		if !ok {
			malformed++
			continue
		}
		if id == o.Ruling {
			matches = append(matches, obj)
			matchRaw = append(matchRaw, line)
			matchLine = append(matchLine, lineNo)
		}
	}
	if malformed > 0 {
		msg := "the ledger has malformed lines, so the search cannot be trusted either way"
		return []Item{
			item("ruling.present", Unknown, "%s (%d malformed)", msg, malformed),
			item("ruling.shape", Unknown, "%s", msg),
		}
	}
	if len(matches) == 0 {
		return []Item{item("ruling.present", Fail, "no ruling with id %q in a well-formed ledger", o.Ruling), item("ruling.shape", Fail, "no such ruling")}
	}
	for _, r := range matchRaw[1:] {
		if !bytes.Equal(r, matchRaw[0]) {
			return []Item{
				item("ruling.present", Unknown, "the id %q appears %d times with different contents (lines %v); read them by hand", o.Ruling, len(matches), matchLine),
				item("ruling.shape", Unknown, "conflicting duplicates"),
			}
		}
	}
	present := item("ruling.present", Pass, "found id %q (%d identical line(s))", o.Ruling, len(matches))
	obj := matches[0]
	when, _ := str(obj["ts"])
	if when == "" {
		when, _ = str(obj["date"])
	}
	by, _ := str(obj["by"])
	if by == "" {
		by, _ = str(obj["ruled_by"])
	}
	okWhen := false
	if when != "" {
		if _, e := time.Parse(time.RFC3339, when); e == nil {
			okWhen = true
		} else if _, e := time.Parse("2006-01-02", when); e == nil {
			okWhen = true
		}
	}
	shape := item("ruling.shape", Pass, "dated %s, by %s; whether the ruling covers the action is not checked", when, by)
	if !okWhen || strings.TrimSpace(by) == "" {
		shape = item("ruling.shape", Fail, "the ruling lacks a parseable ts/date or a by/ruled_by (date %q, by %q)", when, by)
	}
	shape.Data = map[string]any{"date": when, "by": by}
	return []Item{present, shape}
}
