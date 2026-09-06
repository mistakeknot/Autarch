package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Late proposals and notes may still carry a pre-deletion source. Preserve the
// project's authoritative source tombstones when these records arrive.
func preserveSourceTombstones(st *State, project string, sources []Source) []Source {
	deleted := map[string]bool{}
	collect := func(evidence []Source) {
		for _, source := range evidence {
			if source.Status == "deleted" && source.ID != "" {
				deleted[source.ID] = true
			}
		}
	}
	for _, session := range st.Sessions {
		if session.Project == project {
			collect(session.Media)
		}
	}
	for _, note := range st.Feedback {
		if note.Project == project {
			collect(note.Evidence)
		}
	}
	for _, proposal := range st.Proposals {
		if proposal.Project == project {
			collect(proposal.Evidence)
		}
	}
	out := append([]Source(nil), sources...)
	for i, source := range out {
		if deleted[source.ID] {
			out[i].Status, out[i].Path = "deleted", ""
		}
	}
	return out
}

// DeleteCaptures only processes an explicit, revision-bound deletion request.
// Notes and accepted decisions remain; media references become tombstones.
func (s *Store) DeleteCaptures() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.state.Sessions {
		if session.Status != "deleting" && session.Status != "deleted" {
			continue
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(id) {
			return fmt.Errorf("invalid capture directory identity")
		}
		root, err := filepath.EvalSymlinks(s.dir)
		if err != nil {
			return err
		}
		media := filepath.Join(root, "media")
		dir := filepath.Join(media, id)
		if actual, err := filepath.EvalSymlinks(dir); err == nil && actual != dir {
			return fmt.Errorf("refusing symlinked capture directory")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = os.RemoveAll(dir); err != nil {
			return err
		}
		if session.Status == "deleted" {
			continue
		} // reap any delayed companion write
		originalDir, _ := filepath.Abs(filepath.Join(s.dir, "media", id))
		// RemoveAll never follows nested symlinks. Acknowledged source records
		// retain their identities and notes, while live references cannot reopen it.
		next := s.state.Clone()
		deleted := func(sources []Source) []Source {
			for i, source := range sources {
				p, _ := filepath.Abs(source.Path)
				if strings.HasPrefix(p, dir+string(filepath.Separator)) || strings.HasPrefix(p, originalDir+string(filepath.Separator)) {
					sources[i].Status = "deleted"
					sources[i].Path = ""
				}
			}
			return sources
		}
		session.Status = "deleted"
		session.Error = ""
		session.Media = deleted(session.Media)
		session.Revision++
		next.Sessions[id] = session
		for key, f := range next.Feedback {
			if f.SessionID == id {
				f.Evidence = deleted(f.Evidence)
				f.Revision++
				next.Feedback[key] = f
			}
		}
		for key, p := range next.Proposals {
			p.Evidence = deleted(p.Evidence)
			next.Proposals[key] = p
		}
		if err = s.commit(next); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) RunRetention(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := s.DeleteCaptures(); err != nil {
				fmt.Fprintln(os.Stderr, "Capture deletion pending:", err)
			}
		}
	}
}
