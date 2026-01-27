package epics

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ValidationSeverity indicates whether an error is auto-fixable or fatal.
type ValidationSeverity string

const (
	SeverityFixable ValidationSeverity = "fixable" // can be auto-repaired
	SeverityFatal   ValidationSeverity = "fatal"   // requires manual intervention
)

type ValidationError struct {
	Path     string
	Message  string
	Severity ValidationSeverity
	Fix      string // human-readable fix guidance
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
			errs = append(errs, ValidationError{
				Path: path("id"), Message: "invalid epic id",
				Severity: SeverityFixable,
				Fix:      fmt.Sprintf("use format EPIC-NNN (e.g. EPIC-%03d)", i+1),
			})
		} else if seenEpics[epic.ID] {
			errs = append(errs, ValidationError{
				Path: path("id"), Message: "duplicate epic id",
				Severity: SeverityFatal,
				Fix:      "each epic must have a unique ID",
			})
		} else {
			seenEpics[epic.ID] = true
		}
		if epic.Title == "" {
			errs = append(errs, ValidationError{
				Path: path("title"), Message: "title required",
				Severity: SeverityFatal,
				Fix:      "provide a descriptive title for the epic",
			})
		}
		if !validStatus(epic.Status) {
			errs = append(errs, ValidationError{
				Path: path("status"), Message: fmt.Sprintf("invalid status %q", epic.Status),
				Severity: SeverityFixable,
				Fix:      "allowed: todo, in_progress, review, blocked, done",
			})
		}
		if !validPriority(epic.Priority) {
			errs = append(errs, ValidationError{
				Path: path("priority"), Message: fmt.Sprintf("invalid priority %q", epic.Priority),
				Severity: SeverityFixable,
				Fix:      "allowed: p0, p1, p2, p3",
			})
		}
		for j, story := range epic.Stories {
			sp := func(field string) string {
				return "epics[" + strconv.Itoa(i) + "].stories[" + strconv.Itoa(j) + "]." + field
			}
			if story.ID == "" || !storyIDPattern.MatchString(story.ID) {
				errs = append(errs, ValidationError{
					Path: sp("id"), Message: "invalid story id",
					Severity: SeverityFixable,
					Fix:      fmt.Sprintf("use format EPIC-NNN-SNN (e.g. %s-S%02d)", epic.ID, j+1),
				})
			} else if epic.ID != "" && !strings.HasPrefix(story.ID, epic.ID+"-") {
				errs = append(errs, ValidationError{
					Path: sp("id"), Message: "story id must match epic",
					Severity: SeverityFixable,
					Fix:      fmt.Sprintf("prefix with %s- (e.g. %s-S%02d)", epic.ID, epic.ID, j+1),
				})
			} else if seenStories[story.ID] {
				errs = append(errs, ValidationError{
					Path: sp("id"), Message: "duplicate story id",
					Severity: SeverityFatal,
					Fix:      "each story must have a unique ID",
				})
			} else {
				seenStories[story.ID] = true
			}
			if story.Title == "" {
				errs = append(errs, ValidationError{
					Path: sp("title"), Message: "title required",
					Severity: SeverityFatal,
					Fix:      "provide a descriptive title for the story",
				})
			}
			if !validStatus(story.Status) {
				errs = append(errs, ValidationError{
					Path: sp("status"), Message: fmt.Sprintf("invalid status %q", story.Status),
					Severity: SeverityFixable,
					Fix:      "allowed: todo, in_progress, review, blocked, done",
				})
			}
			if !validPriority(story.Priority) {
				errs = append(errs, ValidationError{
					Path: sp("priority"), Message: fmt.Sprintf("invalid priority %q", story.Priority),
					Severity: SeverityFixable,
					Fix:      "allowed: p0, p1, p2, p3",
				})
			}
		}
	}
	return errs
}

// AutoFix attempts to repair fixable validation errors in-place.
// Returns the remaining (unfixable) errors.
func AutoFix(list []Epic) []ValidationError {
	for i := range list {
		epic := &list[i]
		if !epicIDPattern.MatchString(epic.ID) {
			epic.ID = fmt.Sprintf("EPIC-%03d", i+1)
		}
		if !validStatus(epic.Status) {
			epic.Status = StatusTodo
		}
		if !validPriority(epic.Priority) {
			epic.Priority = PriorityP2
		}
		for j := range epic.Stories {
			story := &epic.Stories[j]
			if !storyIDPattern.MatchString(story.ID) || !strings.HasPrefix(story.ID, epic.ID+"-") {
				story.ID = fmt.Sprintf("%s-S%02d", epic.ID, j+1)
			}
			if !validStatus(story.Status) {
				story.Status = StatusTodo
			}
			if !validPriority(story.Priority) {
				story.Priority = PriorityP2
			}
		}
	}
	return Validate(list)
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
		severity := "ERROR"
		if err.Severity == SeverityFixable {
			severity = "FIXABLE"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", severity, err.Path, err.Message)
		if err.Fix != "" {
			fmt.Fprintf(&b, "  fix: %s\n", err.Fix)
		}
	}
	return b.String()
}

// HasFatalErrors returns true if any error is not auto-fixable.
func HasFatalErrors(errs []ValidationError) bool {
	for _, e := range errs {
		if e.Severity == SeverityFatal {
			return true
		}
	}
	return false
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
