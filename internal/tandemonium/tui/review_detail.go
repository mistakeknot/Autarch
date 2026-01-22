package tui

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type ReviewFile struct {
	Path    string
	Added   int
	Deleted int
}

type ReviewDetail struct {
	TaskID             string
	Title              string
	Summary            string
	UserStory          string
	StoryDrift         string
	Alignment          string
	AcceptanceCriteria []string
	Files              []ReviewFile
	TestsSummary       string
}

func LoadReviewDetail(taskID string) (ReviewDetail, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return ReviewDetail{}, err
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		db = nil
	}
	return LoadReviewDetailWithDB(db, taskID)
}

func LoadReviewDetailWithDB(db *sql.DB, taskID string) (ReviewDetail, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return ReviewDetail{}, err
	}
	specPath, err := project.TaskSpecPath(root, taskID)
	if err != nil {
		return ReviewDetail{}, err
	}
	detail, err := specs.LoadDetail(specPath)
	if err != nil {
		return ReviewDetail{}, err
	}
	cfg, err := config.LoadFromProject(root)
	if err != nil {
		return ReviewDetail{}, err
	}
	runner := &git.ExecRunner{}
	base, err := reviewBaseBranch(cfg, runner)
	if err != nil {
		return ReviewDetail{}, err
	}
	branch, err := git.BranchForTask(runner, taskID)
	if err != nil {
		return ReviewDetail{}, err
	}
	stats, err := git.DiffNumstat(runner, base, branch)
	if err != nil {
		return ReviewDetail{}, err
	}
	var files []ReviewFile
	for _, s := range stats {
		files = append(files, ReviewFile{
			Path:    s.Path,
			Added:   s.Added,
			Deleted: s.Deleted,
		})
	}
	testsSummary := "Tests: unknown"
	if db != nil {
		if session, err := storage.FindSessionByTask(db, taskID); err == nil {
			logPath := filepath.Join(project.SessionsDir(root), session.ID+".log")
			if raw, err := os.ReadFile(logPath); err == nil {
				testsSummary = FindTestSummary(string(raw))
			}
		}
	}
	if testsSummary == "" {
		testsSummary = "Tests: unknown"
	}
	storyDrift := "unknown"
	if detail.UserStoryHash != "" && detail.UserStory != "" {
		if detail.UserStoryHash == specs.StoryHash(detail.UserStory) {
			storyDrift = "ok"
		} else {
			storyDrift = "changed"
		}
	}
	alignment := "unknown"
	if detail.MVPIncluded != nil {
		if *detail.MVPIncluded {
			alignment = "mvp"
		} else {
			alignment = "out"
		}
	}
	return ReviewDetail{
		TaskID:             detail.ID,
		Title:              detail.Title,
		Summary:            detail.Summary,
		UserStory:          detail.UserStory,
		StoryDrift:         storyDrift,
		Alignment:          alignment,
		AcceptanceCriteria: detail.AcceptanceCriteria,
		Files:              files,
		TestsSummary:       testsSummary,
	}, nil
}

var ErrNoReviewTask = errors.New("no review task selected")
