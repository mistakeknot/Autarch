package backlogrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State is the run's memory, kept as plain files under one directory.
type State struct{ Dir string }

// PauseFile is the file whose existence stops every run (touch it to pause, remove it to resume).
func (s State) PauseFile() string { return filepath.Join(s.Dir, "PAUSE") }

func (s State) Paused() bool {
	_, err := os.Stat(s.PauseFile())
	return err == nil
}

// Memory is what one run hands the next.
type Memory struct {
	// Verdicts by card key, from the latest weekly triage.
	Verdicts map[string]Verdict `json:"verdicts"`
	// Sent records the last message line sent about a card: what it said and when.
	Sent map[string]Sent `json:"sent"`
	// Seen is the card set of the last weekly snapshot, key to updatedAt.
	Seen map[string]string `json:"seen"`
}

type Sent struct {
	Kind string    `json:"kind"` // ready, needs-spec, gated:<home state>
	At   time.Time `json:"at"`
	// UpdatedAt is the card's updatedAt when it was sent. A ready card is sent once per version of the card.
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func (s State) Load() (Memory, error) {
	m := Memory{Verdicts: map[string]Verdict{}, Sent: map[string]Sent{}, Seen: map[string]string{}}
	b, err := os.ReadFile(filepath.Join(s.Dir, "memory.json"))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("memory.json unreadable (move it aside to start fresh): %w", err)
	}
	if m.Verdicts == nil {
		m.Verdicts = map[string]Verdict{}
	}
	if m.Sent == nil {
		m.Sent = map[string]Sent{}
	}
	if m.Seen == nil {
		m.Seen = map[string]string{}
	}
	return m, nil
}

// Save writes the memory atomically.
func (s State) Save(m Memory) error {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.Dir, "memory.json"), b)
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Receipt is the one line each run appends.
type Receipt struct {
	At       time.Time      `json:"at"`
	Mode     string         `json:"mode"`
	DryRun   bool           `json:"dry_run"`
	Outcome  string         `json:"outcome"` // ok, paused, skipped, failed
	Detail   string         `json:"detail,omitempty"`
	Seat     *Seat          `json:"seat,omitempty"`
	Counts   map[string]int `json:"counts,omitempty"`
	Digest   string         `json:"digest,omitempty"`
	Messaged []string       `json:"messaged,omitempty"`
}

func (s State) AppendReceipt(r Receipt) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, "receipts.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
