// Package review provides multi-agent PRD quality validation for Gurgeh.
// Following the Compound Engineering pattern, multiple specialized reviewers
// analyze PRDs in parallel to catch issues before implementation begins.
package review

import (
	"context"
	"sync"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// PRDReviewer validates PRD quality from a specific perspective.
type PRDReviewer interface {
	// Review analyzes a PRD and returns findings.
	Review(ctx context.Context, spec *specs.Spec) (*ReviewResult, error)

	// Name returns the reviewer's identifier.
	Name() string
}

// ReviewResult captures validation findings from a single reviewer.
type ReviewResult struct {
	// Reviewer identifies which reviewer produced this result.
	Reviewer string

	// Score indicates overall quality (0.0-1.0).
	Score float64

	// Issues are problems that should be addressed.
	Issues []Issue

	// Suggestions are optional improvements.
	Suggestions []string
}

// Issue represents a problem found during review.
type Issue struct {
	// Severity indicates how serious the issue is.
	Severity IssueSeverity

	// Category classifies the type of issue.
	Category string

	// Description explains the problem.
	Description string

	// Location points to where in the PRD the issue was found.
	Location string
}

// IssueSeverity indicates how serious an issue is.
type IssueSeverity string

const (
	SeverityError   IssueSeverity = "error"   // Must fix before implementation
	SeverityWarning IssueSeverity = "warning" // Should fix
	SeverityInfo    IssueSeverity = "info"    // Nice to fix
)

// CombinedReview aggregates results from multiple reviewers.
type CombinedReview struct {
	// SpecID identifies the reviewed PRD.
	SpecID string

	// Results from each reviewer.
	Results []*ReviewResult

	// OverallScore is the weighted average of all reviewer scores.
	OverallScore float64

	// TotalIssues counts all issues across reviewers.
	TotalIssues int

	// Errors counts issues with error severity.
	Errors int

	// Warnings counts issues with warning severity.
	Warnings int

	// ReadyForImplementation indicates if the PRD can proceed.
	ReadyForImplementation bool
}

// PassThreshold is the minimum overall score to pass review.
const PassThreshold = 0.7

// RunParallelReview runs all reviewers concurrently and combines results.
func RunParallelReview(ctx context.Context, spec *specs.Spec, reviewers []PRDReviewer) (*CombinedReview, error) {
	combined := &CombinedReview{
		SpecID:  spec.ID,
		Results: make([]*ReviewResult, 0, len(reviewers)),
	}

	// Run reviewers in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	resultsCh := make(chan *ReviewResult, len(reviewers))
	errorsCh := make(chan error, len(reviewers))

	for _, reviewer := range reviewers {
		wg.Add(1)
		go func(r PRDReviewer) {
			defer wg.Done()

			result, err := r.Review(ctx, spec)
			if err != nil {
				errorsCh <- err
				return
			}
			resultsCh <- result
		}(reviewer)
	}

	// Wait for all reviewers to complete
	wg.Wait()
	close(resultsCh)
	close(errorsCh)

	// Check for errors
	select {
	case err := <-errorsCh:
		if err != nil {
			return nil, err
		}
	default:
	}

	// Collect results
	var totalScore float64
	for result := range resultsCh {
		mu.Lock()
		combined.Results = append(combined.Results, result)
		totalScore += result.Score
		for _, issue := range result.Issues {
			combined.TotalIssues++
			switch issue.Severity {
			case SeverityError:
				combined.Errors++
			case SeverityWarning:
				combined.Warnings++
			}
		}
		mu.Unlock()
	}

	// Calculate overall score
	if len(combined.Results) > 0 {
		combined.OverallScore = totalScore / float64(len(combined.Results))
	}
	combined.ReadyForImplementation = combined.OverallScore >= PassThreshold && combined.Errors == 0

	return combined, nil
}

// DefaultReviewers returns the standard set of PRD reviewers.
func DefaultReviewers() []PRDReviewer {
	return []PRDReviewer{
		NewCompletenessReviewer(),
		NewCUJConsistencyReviewer(),
		NewAcceptanceCriteriaReviewer(),
		NewScopeCreepDetector(),
	}
}
