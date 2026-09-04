package door

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The briefing axis (autarch-01, step 1): what moved in each garden since mk
// last opened Autarch. Same discipline as check.go and sessions.go -- pure
// resolution tested on fixtures, a thin exec wrapper over the garden's own
// git, and an explicit could-not-read state. A garden whose git call failed
// carries Err and is named as unread; it is never shown as a garden where
// nothing moved, because "nothing happened" and "could not look" must not
// print the same.

// Commit is one commit that landed on any ref inside the window.
type Commit struct {
	Hash    string
	Subject string
	When    time.Time // committer date: when it landed here
}

// Movement is what one garden did while mk was away.
type Movement struct {
	Root  string
	Name  string
	Since time.Time

	Commits  []Commit  // newest first, across every ref, capped at maxCommits
	Dirty    int       // lines of `status --porcelain`: work in flight, untracked included
	Sessions int       // Claude Code transcript files touched inside the window
	Latest   time.Time // newest commit or transcript touch; zero when nothing moved

	Err error // the garden could not be read; the counts above are then meaningless
}

// Moved reports whether the garden earns a line on the briefing. The default
// is quiet: a garden with nothing to say is omitted, not listed at zero.
func (m Movement) Moved() bool {
	return m.Err == nil && (len(m.Commits) > 0 || m.Dirty > 0 || m.Sessions > 0)
}

// movementTimeout bounds the two git calls for one garden. git answers a
// local repository in well under a second; ten seconds means something is
// wrong enough to report as unread.
const movementTimeout = 10 * time.Second

// maxCommits caps the log per garden. The briefing shows a count and the
// newest subject; two hundred is more than any line will ever render.
const maxCommits = 200

// gitSinceLayout is ISO 8601 with a compact offset, which every git parses.
const gitSinceLayout = "2006-01-02T15:04:05-0700"

// GitMovement asks the garden's own git what landed inside the window and
// what is in flight now. --all covers every ref, so a lane an agent pushed
// from another machine and this clone fetched counts as movement even when
// main never moved; --since filters on the committer date, which is when the
// work arrived here.
func GitMovement(ctx context.Context, p Project, since time.Time) Movement {
	m := Movement{Root: p.Root, Name: p.Name, Since: since}
	ctx, cancel := context.WithTimeout(ctx, movementTimeout)
	defer cancel()

	gitBin, err := exec.LookPath("git")
	if err != nil {
		m.Err = fmt.Errorf("git not on PATH: %w", err)
		return m
	}

	out, err := exec.CommandContext(ctx, gitBin, "-C", p.Root, "log", "--all",
		"--since="+since.Format(gitSinceLayout),
		"--format=%H%x1f%ct%x1f%s", "-n", strconv.Itoa(maxCommits)).Output()
	if err != nil {
		m.Err = gitErr(ctx, "log", p.Name, err)
		return m
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			m.Err = fmt.Errorf("git log %s: unparseable line %q", p.Name, line)
			return m
		}
		secs, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			m.Err = fmt.Errorf("git log %s: bad timestamp in %q", p.Name, line)
			return m
		}
		c := Commit{Hash: parts[0], Subject: parts[2], When: time.Unix(secs, 0)}
		m.Commits = append(m.Commits, c)
		if c.When.After(m.Latest) {
			m.Latest = c.When
		}
	}
	sort.SliceStable(m.Commits, func(i, j int) bool { return m.Commits[i].When.After(m.Commits[j].When) })

	out, err = exec.CommandContext(ctx, gitBin, "-C", p.Root, "status", "--porcelain").Output()
	if err != nil {
		m.Err = gitErr(ctx, "status", p.Name, err)
		return m
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			m.Dirty++
		}
	}
	return m
}

// gitErr names the failure with git's own stderr when there is one.
func gitErr(ctx context.Context, sub, name string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("git %s timed out on %s", sub, name)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("git %s %s: %s", sub, name, strings.TrimSpace(string(ee.Stderr)))
	}
	return fmt.Errorf("git %s %s: %w", sub, name, err)
}

// DefaultTranscriptsRoot is where Claude Code keeps per-project transcripts:
// one directory per working directory, named by encodeTranscriptDir.
func DefaultTranscriptsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// encodeTranscriptDir mirrors Claude Code's naming: every "/" and "." in the
// absolute working directory becomes "-", so /a/b/.c becomes -a-b--c.
func encodeTranscriptDir(root string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(root)
}

// SessionStat is one garden's share of the transcripts root.
type SessionStat struct {
	Files  int       // transcript files touched inside the window
	Latest time.Time // newest touch
}

// IndexSessions attributes every transcript directory under transcriptsRoot to
// a garden and counts the transcripts whose last real turn falls inside the
// window. Not mtime: bookkeeping rows touch every transcript daily, so mtime
// read every one of mk's 33 threads as moved today (probe finding 4,
// 2026-09-02); the last user or assistant turn is the clock. A directory
// belongs to the garden whose encoded root is its longest matching prefix --
// exact, or followed by "-" -- so a session run inside a nested repo rolls up
// to the garden containing it, and a garden named foo cannot absorb its
// sibling foo-bar. Gardens with no directory are absent from the map: that is
// a real zero. A root that cannot be read is an error: that is a failure to
// look, and the caller must say so once for the estate rather than once per
// row.
func IndexSessions(transcriptsRoot string, projects []Project, since time.Time) (map[string]SessionStat, error) {
	entries, err := os.ReadDir(transcriptsRoot)
	if err != nil {
		return nil, fmt.Errorf("transcripts root %s: %w", transcriptsRoot, err)
	}
	type owner struct{ dir, root string }
	owners := make([]owner, 0, len(projects))
	for _, p := range projects {
		owners = append(owners, owner{dir: encodeTranscriptDir(p.Root), root: p.Root})
	}
	sort.Slice(owners, func(i, j int) bool { return len(owners[i].dir) > len(owners[j].dir) })

	idx := make(map[string]SessionStat)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := ""
		for _, o := range owners {
			if e.Name() == o.dir || strings.HasPrefix(e.Name(), o.dir+"-") {
				root = o.root
				break
			}
		}
		if root == "" {
			continue
		}
		files, err := os.ReadDir(filepath.Join(transcriptsRoot, e.Name()))
		if err != nil {
			continue
		}
		stat := idx[root]
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			last, err := LastTurn(filepath.Join(transcriptsRoot, e.Name(), f.Name()), lastTurnTailBytes)
			if err != nil || last.IsZero() {
				continue // unreadable, or no real turn in the tail: not a movement
			}
			if last.After(since) {
				stat.Files++
				if last.After(stat.Latest) {
					stat.Latest = last
				}
			}
		}
		if stat.Files > 0 {
			idx[root] = stat
		}
	}
	return idx, nil
}

// Movements reads the estate: git per garden with bounded parallelism, the
// session index merged in, each finished garden streamed to onResult from a
// worker goroutine (the model receives them over a channel, so order is by
// completion). Blocks until every garden has been reported.
func Movements(ctx context.Context, projects []Project, since time.Time, idx map[string]SessionStat, onResult func(Movement)) {
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range projects {
		wg.Add(1)
		go func(p Project) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			m := GitMovement(ctx, p, since)
			if s, ok := idx[p.Root]; ok && m.Err == nil {
				m.Sessions = s.Files
				if s.Latest.After(m.Latest) {
					m.Latest = s.Latest
				}
			}
			onResult(m)
		}(projects[i])
	}
	wg.Wait()
}
