package door

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Transcript reading (autarch-01 step 2, probe findings 3 and 4). mtime is
// not a liveness signal for Claude Code transcripts -- every one of them
// reads as touched today because of untimestamped bookkeeping rows appended
// on every turn. The last real conversational turn is; this file finds it,
// and finds which gardens a root-launched thread actually touched, from the
// paths inside the transcript rather than from its directory (the directory
// attribution IndexSessions already does sees only 2 of 33 live threads).

// lastTurnTailBytes bounds how much of a transcript LastTurn reads: the tail
// carries the newest turns, and transcripts run tens of megabytes.
const lastTurnTailBytes = 256 << 10

// gardensScanBytes bounds how much of a transcript Gardens reads: wide
// enough to catch a standing thread's whole day of garden-hopping (the probe
// scanned 1,010 MB across 33 transcripts in about a minute at this size).
const gardensScanBytes = 16 << 20

// FindTranscript locates <transcriptsRoot>/*/<id>.jsonl. Several matches
// (the id resolved under more than one encoded directory) is a real
// possibility -- Claude Code's directory encoding can collide -- and the
// newest ModTime wins, since that is the transcript still being written to.
func FindTranscript(transcriptsRoot, id string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(transcriptsRoot, "*", id+".jsonl"))
	if err != nil {
		return "", fmt.Errorf("find transcript %s: %w", id, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no transcript for %s under %s", id, transcriptsRoot)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	best := matches[0]
	var bestTime time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if mt := info.ModTime(); mt.After(bestTime) {
			bestTime = mt
			best = m
		}
	}
	return best, nil
}

// turnLine is the only shape LastTurn cares about: bookkeeping rows
// (bridge-session, mode, permission-mode, last-prompt, atis-latch, system/*)
// either carry no Timestamp or a Type outside {user, assistant}, so they
// never satisfy the turn test below regardless of what else is on the line.
type turnLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
}

// LastTurn is the timestamp of the last entry whose type is "user" or
// "assistant" within the trailing tailBytes of path. Bookkeeping rows never
// count. Reads only the tail; returns the zero time with a nil error when no
// turn is found there (a real fact -- the window is too narrow or the
// transcript is all bookkeeping -- not a failure to look).
func LastTurn(path string, tailBytes int64) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return time.Time{}, err
	}
	offset := int64(0)
	if size := info.Size(); size > tailBytes {
		offset = size - tailBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return time.Time{}, err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), gardensLineBuffer)
	discardFirst := offset > 0 // the seek can land mid-line; that fragment is not a turn
	var last time.Time
	for scanner.Scan() {
		if discardFirst {
			discardFirst = false
			continue
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var tl turnLine
		if err := json.Unmarshal(line, &tl); err != nil {
			continue
		}
		if tl.Type != "user" && tl.Type != "assistant" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, tl.Timestamp)
		if err != nil {
			continue
		}
		last = ts
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, err
	}
	return last, nil
}

// GardenHit is one garden mentioned by absolute path inside a transcript.
type GardenHit struct {
	Root     string
	Name     string
	Mentions int
}

// gardensLineBuffer bounds one scanned transcript line; a whole JSON turn on
// one line routinely exceeds bufio.Scanner's 64 KiB default.
const gardensLineBuffer = 4 << 20

// topLevelGardenRoots is the estate-root set Gardens scans for: the project
// roots not nested inside another project root in the same list. A path
// under a nested project (Sylveste/apps/Autarch) is still caught -- the
// ancestor's pattern matches it too -- and attribution below walks every
// project root longest-first to credit the actual owner, so building a
// pattern for the nested root as well would only double-count the same
// mention.
func topLevelGardenRoots(projects []Project) []string {
	var roots []string
	for _, p := range projects {
		nested := false
		for _, q := range projects {
			if q.Root != p.Root && strings.HasPrefix(p.Root, q.Root+string(filepath.Separator)) {
				nested = true
				break
			}
		}
		if !nested {
			roots = append(roots, p.Root)
		}
	}
	return roots
}

// Gardens counts mentions of every project root inside the trailing
// scanBytes of the transcript at path. A path under a nested project
// (Sylveste/apps/Autarch) credits the longest matching root only. Sorted by
// Mentions desc, then Name. A transcript with no project paths (a non-code
// garden like taxes) returns an empty, non-nil-error slice -- absence is a
// measurement, not a failure to look.
func Gardens(path string, projects []Project, scanBytes int64) ([]GardenHit, error) {
	roots := topLevelGardenRoots(projects)
	if len(roots) == 0 {
		return nil, nil
	}
	parts := make([]string, len(roots))
	for i, r := range roots {
		parts[i] = regexp.QuoteMeta(r)
	}
	// One combined pattern over every estate root in a single scan pass;
	// the trailing "/" boundary keeps a sibling whose name prefixes another
	// (foo vs foo-bar) from cross-matching, same guard as ResolveSessions.
	pathPattern := regexp.MustCompile(`(?:` + strings.Join(parts, "|") + `)/[^"'\\\s]+`)

	nameByRoot := make(map[string]string, len(projects))
	longestFirst := make([]string, 0, len(projects))
	for _, p := range projects {
		nameByRoot[p.Root] = p.Name
		longestFirst = append(longestFirst, p.Root)
	}
	sort.Slice(longestFirst, func(i, j int) bool { return len(longestFirst[i]) > len(longestFirst[j]) })

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := int64(0)
	if size := info.Size(); size > scanBytes {
		offset = size - scanBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), gardensLineBuffer)
	discardFirst := offset > 0
	for scanner.Scan() {
		if discardFirst {
			discardFirst = false
			continue
		}
		for _, m := range pathPattern.FindAll(scanner.Bytes(), -1) {
			mp := string(m)
			for _, r := range longestFirst {
				if mp == r || strings.HasPrefix(mp, r+string(filepath.Separator)) {
					counts[r]++
					break
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return nil, nil
	}
	hits := make([]GardenHit, 0, len(counts))
	for r, n := range counts {
		hits = append(hits, GardenHit{Root: r, Name: nameByRoot[r], Mentions: n})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Mentions != hits[j].Mentions {
			return hits[i].Mentions > hits[j].Mentions
		}
		return hits[i].Name < hits[j].Name
	})
	return hits, nil
}
