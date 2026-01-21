package tui

import (
	"fmt"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

const diffPageSize = 16

type ReviewDiffState struct {
	Files      []string
	Current    int
	Lines      []string
	Cache      map[string][]string
	Offsets    map[string]int
	Offset     int
	BaseBranch string
	TaskBranch string
	Loader     func(path string) ([]string, error)
}

func LoadReviewDiff(taskID string) (ReviewDiffState, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return ReviewDiffState{}, err
	}
	cfg, err := config.LoadFromProject(root)
	if err != nil {
		return ReviewDiffState{}, err
	}
	runner := &git.ExecRunner{}
	base, err := reviewBaseBranch(cfg, runner)
	if err != nil {
		return ReviewDiffState{}, err
	}
	taskBranch, err := git.BranchForTask(runner, taskID)
	if err != nil {
		return ReviewDiffState{}, err
	}
	files, err := git.DiffNameOnlyRange(runner, base, taskBranch)
	if err != nil {
		return ReviewDiffState{}, err
	}
	return buildReviewDiffState(base, taskBranch, files, func(path string) ([]string, error) {
		return git.DiffUnified(runner, base, taskBranch, path)
	})
}

func buildReviewDiffState(base, branch string, files []string, diff func(path string) ([]string, error)) (ReviewDiffState, error) {
	state := ReviewDiffState{
		Files:      files,
		Current:    0,
		Cache:      map[string][]string{},
		Offsets:    map[string]int{},
		Offset:     0,
		BaseBranch: base,
		TaskBranch: branch,
		Loader:     diff,
	}
	for _, path := range files {
		state.Offsets[path] = 0
	}
	if len(files) > 0 {
		lines, err := diff(files[0])
		if err != nil {
			return ReviewDiffState{}, err
		}
		state.Cache[files[0]] = lines
		state.Lines = lines
	}
	return state, nil
}

func currentBranch(runner git.Runner) (string, error) {
	out, err := runner.Run("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func reviewBaseBranch(cfg config.Config, runner git.Runner) (string, error) {
	base := strings.TrimSpace(cfg.Review.TargetBranch)
	if base != "" {
		return base, nil
	}
	return currentBranch(runner)
}

func (m Model) viewReviewDiff() string {
	total := len(m.ReviewDiff.Files)
	out := "REVIEW DIFF"
	if total > 0 {
		out += " " + fmt.Sprintf("(%d/%d)", m.ReviewDiff.Current+1, total)
	}
	out += "\n\n"
	if total == 0 {
		out += "No diff files.\n\n[b]ack\n"
		return out
	}
	path := m.ReviewDiff.Files[m.ReviewDiff.Current]
	out += "FILE: " + path + "\n"
	if m.ReviewDiff.BaseBranch != "" || m.ReviewDiff.TaskBranch != "" {
		out += "BASE: " + m.ReviewDiff.BaseBranch + "  BRANCH: " + m.ReviewDiff.TaskBranch + "\n"
	}
	out += "\n"
	lines := m.ReviewDiff.Lines
	start := m.ReviewDiff.Offset
	if start < 0 {
		start = 0
	}
	end := start + diffPageSize
	if end > len(lines) {
		end = len(lines)
	}
	for i := start; i < end; i++ {
		out += lines[i] + "\n"
	}
	out += "\n[j/k] next/prev  [u/d] page  [g/G] top/bottom  [b]ack\n"
	return out
}

func (m *Model) handleReviewDiffKey(key string) {
	if len(m.ReviewDiff.Files) == 0 {
		return
	}
	switch key {
	case "j", "down":
		if m.ReviewDiff.Current < len(m.ReviewDiff.Files)-1 {
			m.setReviewDiffCurrent(m.ReviewDiff.Current + 1)
		}
	case "k", "up":
		if m.ReviewDiff.Current > 0 {
			m.setReviewDiffCurrent(m.ReviewDiff.Current - 1)
		}
	case "u":
		m.adjustReviewDiffOffset(-diffPageSize)
	case "d":
		m.adjustReviewDiffOffset(diffPageSize)
	case "g":
		m.setReviewDiffOffset(0)
	case "G":
		m.setReviewDiffOffset(len(m.ReviewDiff.Lines))
	}
}

func (m *Model) setReviewDiffCurrent(idx int) {
	if idx < 0 || idx >= len(m.ReviewDiff.Files) {
		return
	}
	currentPath := m.ReviewDiff.Files[m.ReviewDiff.Current]
	if m.ReviewDiff.Offsets != nil {
		m.ReviewDiff.Offsets[currentPath] = m.ReviewDiff.Offset
	}
	m.ReviewDiff.Current = idx
	path := m.ReviewDiff.Files[idx]
	lines, ok := m.ReviewDiff.Cache[path]
	if !ok {
		if m.ReviewDiff.Loader != nil {
			loaded, err := m.ReviewDiff.Loader(path)
			if err != nil {
				m.SetStatusError(err.Error())
				m.ReviewDiff.Lines = nil
			} else {
				m.ReviewDiff.Cache[path] = loaded
				lines = loaded
			}
		}
	}
	m.ReviewDiff.Lines = lines
	if m.ReviewDiff.Offsets != nil {
		m.ReviewDiff.Offset = m.ReviewDiff.Offsets[path]
	} else {
		m.ReviewDiff.Offset = 0
	}
}

func (m *Model) adjustReviewDiffOffset(delta int) {
	m.setReviewDiffOffset(m.ReviewDiff.Offset + delta)
}

func (m *Model) setReviewDiffOffset(offset int) {
	if offset < 0 {
		offset = 0
	}
	maxStart := len(m.ReviewDiff.Lines) - diffPageSize
	if maxStart < 0 {
		maxStart = 0
	}
	if offset > maxStart {
		offset = maxStart
	}
	m.ReviewDiff.Offset = offset
	path := m.ReviewDiff.Files[m.ReviewDiff.Current]
	if m.ReviewDiff.Offsets != nil {
		m.ReviewDiff.Offsets[path] = offset
	}
}
