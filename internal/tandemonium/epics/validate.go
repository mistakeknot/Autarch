package epics

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ValidationError struct {
	Path    string
	Message string
}

var epicIDPattern = regexp.MustCompile(`^EPIC-\d{3}$`)
var storyIDPattern = regexp.MustCompile(`^EPIC-\d{3}-S\d{2}$`)

func Validate(list []Epic) []ValidationError {
	var errs []ValidationError
	seenEpics := map[string]bool{}
	seenStories := map[string]bool{}
	for i, epic := range list {
		path := func(field string) string { return "epics[" + strconv.Itoa(i) + "]." + field }
		if epic.ID == "" || !epicIDPattern.MatchString(epic.ID) {
			errs = append(errs, ValidationError{Path: path("id"), Message: "invalid epic id"})
		} else if seenEpics[epic.ID] {
			errs = append(errs, ValidationError{Path: path("id"), Message: "duplicate epic id"})
		} else {
			seenEpics[epic.ID] = true
		}
		if epic.Title == "" {
			errs = append(errs, ValidationError{Path: path("title"), Message: "title required"})
		}
		if !validStatus(epic.Status) {
			errs = append(errs, ValidationError{Path: path("status"), Message: "invalid status"})
		}
		if !validPriority(epic.Priority) {
			errs = append(errs, ValidationError{Path: path("priority"), Message: "invalid priority"})
		}
		for j, story := range epic.Stories {
			sp := func(field string) string {
				return "epics[" + strconv.Itoa(i) + "].stories[" + strconv.Itoa(j) + "]." + field
			}
			if story.ID == "" || !storyIDPattern.MatchString(story.ID) {
				errs = append(errs, ValidationError{Path: sp("id"), Message: "invalid story id"})
			} else if epic.ID != "" && !strings.HasPrefix(story.ID, epic.ID+"-") {
				errs = append(errs, ValidationError{Path: sp("id"), Message: "story id must match epic"})
			} else if seenStories[story.ID] {
				errs = append(errs, ValidationError{Path: sp("id"), Message: "duplicate story id"})
			} else {
				seenStories[story.ID] = true
			}
			if story.Title == "" {
				errs = append(errs, ValidationError{Path: sp("title"), Message: "title required"})
			}
			if !validStatus(story.Status) {
				errs = append(errs, ValidationError{Path: sp("status"), Message: "invalid status"})
			}
			if !validPriority(story.Priority) {
				errs = append(errs, ValidationError{Path: sp("priority"), Message: "invalid priority"})
			}
		}
	}
	return errs
}

func WriteValidationReport(dir string, raw []byte, errs []ValidationError) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	outPath := filepath.Join(dir, "init-epics-output.yaml")
	errPath := filepath.Join(dir, "init-epics-errors.txt")
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(errPath, []byte(FormatValidationErrors(errs)), 0o644); err != nil {
		return "", "", err
	}
	return outPath, errPath, nil
}

func FormatValidationErrors(errs []ValidationError) string {
	var b strings.Builder
	for _, err := range errs {
		fmt.Fprintf(&b, "%s: %s\n", err.Path, err.Message)
	}
	return b.String()
}

func validStatus(s Status) bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusReview, StatusBlocked, StatusDone:
		return true
	default:
		return false
	}
}

func validPriority(p Priority) bool {
	switch p {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	default:
		return false
	}
}
