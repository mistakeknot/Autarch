package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/door"
)

type threadsOptions struct {
	roots    []string
	registry string
	jsonOut  bool
}

// threadsCmd lists the registry without the TUI: the same read the threads
// screen does, printed once. It exists so a real render over the estate can
// be checked by a machine (plan WI-8) and so mk can diff the note before
// deleting it.
func threadsCmd() *cobra.Command {
	var o threadsOptions
	cmd := &cobra.Command{
		Use:   "threads",
		Short: "List every live thread on the estate: seat, runtime, last real turn, gardens touched",
		Long: `Reads the tmux session list (name and pane command), classifies what each
pane runs, and for every Claude Code thread carrying a resume id reads its
transcript's last real turn and the gardens it mentions. Nothing is written.

With --registry, a note in the session-name format is compared to the live
seats and every disagreement is printed after the list.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runThreads(o) },
	}
	cmd.Flags().StringArrayVar(&o.roots, "root", nil, "estate root to scan (repeatable; default ~/projects)")
	cmd.Flags().StringVar(&o.registry, "registry", "", "a note in the session-name format to diff against the live seats")
	cmd.Flags().BoolVar(&o.jsonOut, "json", false, "print {threads, drift} as JSON")
	return cmd
}

type threadJSON struct {
	Session      string            `json:"session"`
	Terminal     string            `json:"terminal"`
	Mark         string            `json:"mark"`
	Topic        string            `json:"topic"`
	ResumeID     string            `json:"resume_id"`
	Runtime      string            `json:"runtime"`
	Version      string            `json:"version,omitempty"`
	Activity     int64             `json:"activity"`
	Path         string            `json:"path"`
	Transcript   string            `json:"transcript,omitempty"`
	LastTurn     string            `json:"last_turn,omitempty"`
	Gardens      []door.GardenHit  `json:"gardens"`
	Conversation door.Conversation `json:"conversation"`
	Err          string            `json:"err,omitempty"`
}

func toThreadJSON(th door.Thread) threadJSON {
	j := threadJSON{
		Session:      th.Session,
		Terminal:     th.Seat.Terminal,
		Mark:         string(th.Seat.Mark),
		Topic:        th.Seat.Topic,
		ResumeID:     th.Seat.ResumeID,
		Runtime:      string(th.Runtime),
		Version:      th.Version,
		Activity:     th.Activity,
		Path:         th.Path,
		Transcript:   th.Transcript,
		Gardens:      th.Gardens,
		Conversation: th.Conversation,
	}
	if j.Gardens == nil {
		j.Gardens = []door.GardenHit{}
	}
	if !th.LastTurn.IsZero() {
		j.LastTurn = th.LastTurn.UTC().Format(time.RFC3339)
	}
	if th.Err != nil {
		j.Err = th.Err.Error()
	}
	return j
}

func runThreads(o threadsOptions) error {
	roots := o.roots
	if len(roots) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("no --root and no home directory: %w", err)
		}
		roots = []string{filepath.Join(home, "projects")}
	}
	projects, err := door.DiscoverProjects(roots)
	if err != nil {
		return err
	}
	ctx := context.Background()
	sessions, err := door.ListSessions(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "threads: UNCHECKED (%v)\n", err)
		os.Exit(2)
	}

	var mu sync.Mutex
	var threads []door.Thread
	door.ReadThreadsWithCodex(ctx, sessions, projects, roots, door.DefaultTranscriptsRoot(), door.DefaultCodexRoot(), func(th door.Thread) {
		mu.Lock()
		threads = append(threads, th)
		mu.Unlock()
	})
	door.SortThreads(threads)
	set := door.ThreadSet{Threads: threads, ByRoot: door.Attribute(threads, projects, door.ThreadsMinShare)}

	drift := []door.Drift{}
	if o.registry != "" {
		f, err := os.Open(o.registry)
		if err != nil {
			return err
		}
		note, err := door.ParseRegistryNote(f)
		f.Close()
		if err != nil {
			return err
		}
		drift = door.DiffRegistry(note, threads)
	}

	if o.jsonOut {
		out := struct {
			Threads []threadJSON `json:"threads"`
			Drift   []door.Drift `json:"drift"`
		}{Threads: []threadJSON{}, Drift: drift}
		for _, th := range threads {
			out.Threads = append(out.Threads, toThreadJSON(th))
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	now := time.Now()
	fmt.Println(door.ThreadsHeader(set, 0))
	for _, th := range threads {
		fmt.Println(door.ThreadLine(th, now, 160))
	}
	if o.registry != "" {
		fmt.Printf("\nregistry: %s · %d drift\n", o.registry, len(drift))
		for _, d := range drift {
			fmt.Println("  " + door.DriftLine(d))
		}
	}
	return nil
}
