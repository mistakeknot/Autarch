package consistency

import (
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

// Package-level compiled regexes to avoid recompilation on every call
var (
	soloPatterns       = regexp.MustCompile(`(?i)(solo|individual|single|personal|indie)`)
	enterprisePatterns = regexp.MustCompile(`(?i)(enterprise|admin|sso|100\+|team management|organization)`)
	aiPattern          = regexp.MustCompile(`(?i)\bAI\b|artificial intelligence|machine learning|LLM|GPT`)
	mobileAppPattern   = regexp.MustCompile(`(?i)mobile app|ios|android|native app`)
	socialPattern      = regexp.MustCompile(`(?i)social features|sharing|followers|friends`)
	onboardingGoal     = regexp.MustCompile(`(?i)fast\s+onboarding|quick\s+start|under\s+\d+\s+minutes?`)
	onboardingFeature  = regexp.MustCompile(`(?i)onboarding|signup|getting started`)
	mobileGoal         = regexp.MustCompile(`(?i)mobile|responsive|cross-platform`)
	mobileFeature      = regexp.MustCompile(`(?i)mobile|responsive|ios|android`)
	accountFeature     = regexp.MustCompile(`(?i)signup|registration|create account|login`)
	authFeature        = regexp.MustCompile(`(?i)auth|login|session|token`)
	offlineFeature     = regexp.MustCompile(`(?i)offline|local-first|sync`)
)

// Checker detects a specific type of consistency issue
type Checker interface {
	Check(state *arbiter.SprintState) []arbiter.Conflict
	Name() string
}

// Engine runs all consistency checkers
type Engine struct {
	checkers []Checker
}

// NewEngine creates an engine with all default checkers
func NewEngine() *Engine {
	return &Engine{
		checkers: []Checker{
			&UserFeatureChecker{},
			&GoalFeatureChecker{},
			&ScopeCreepChecker{},
			&AssumptionChecker{},
		},
	}
}

// Check runs all checkers and returns combined conflicts
func (e *Engine) Check(state *arbiter.SprintState) []arbiter.Conflict {
	if state == nil {
		return nil
	}
	var all []arbiter.Conflict
	for _, c := range e.checkers {
		all = append(all, c.Check(state)...)
	}
	return all
}

// UserFeatureChecker detects when features don't match target users
type UserFeatureChecker struct{}

func (c *UserFeatureChecker) Name() string { return "user-feature" }

func (c *UserFeatureChecker) Check(state *arbiter.SprintState) []arbiter.Conflict {
	users := state.Sections[arbiter.PhaseUsers].Content
	features := state.Sections[arbiter.PhaseFeaturesGoals].Content

	if users == "" || features == "" {
		return nil
	}

	var conflicts []arbiter.Conflict

	isSoloUser := soloPatterns.MatchString(users)
	hasEnterpriseFeatures := enterprisePatterns.MatchString(features)

	if isSoloUser && hasEnterpriseFeatures {
		conflicts = append(conflicts, arbiter.Conflict{
			Type:     arbiter.ConflictUserFeature,
			Severity: arbiter.SeverityBlocker,
			Message:  "Features include enterprise capabilities but target users are individuals/solo developers",
			Sections: []arbiter.Phase{arbiter.PhaseUsers, arbiter.PhaseFeaturesGoals},
		})
	}

	return conflicts
}

// GoalFeatureChecker detects when goals aren't supported by features
type GoalFeatureChecker struct{}

func (c *GoalFeatureChecker) Name() string { return "goal-feature" }

func (c *GoalFeatureChecker) Check(state *arbiter.SprintState) []arbiter.Conflict {
	features := state.Sections[arbiter.PhaseFeaturesGoals].Content

	if features == "" {
		return nil
	}

	var conflicts []arbiter.Conflict

	// Check for goal keywords without supporting features
	goalPatterns := []struct {
		goal    string
		goalRe  *regexp.Regexp
		featRe  *regexp.Regexp
	}{
		{"fast onboarding", onboardingGoal, onboardingFeature},
		{"mobile support", mobileGoal, mobileFeature},
	}

	for _, gp := range goalPatterns {
		hasGoal := gp.goalRe.MatchString(features)
		hasFeature := gp.featRe.MatchString(features)

		if hasGoal && !hasFeature {
			conflicts = append(conflicts, arbiter.Conflict{
				Type:     arbiter.ConflictGoalFeature,
				Severity: arbiter.SeverityWarning,
				Message:  "Goal mentions '" + gp.goal + "' but no supporting features found",
				Sections: []arbiter.Phase{arbiter.PhaseFeaturesGoals},
			})
		}
	}

	return conflicts
}

// ScopeCreepChecker detects features that contradict non-goals
type ScopeCreepChecker struct{}

func (c *ScopeCreepChecker) Name() string { return "scope-creep" }

func (c *ScopeCreepChecker) Check(state *arbiter.SprintState) []arbiter.Conflict {
	features := state.Sections[arbiter.PhaseFeaturesGoals].Content
	scope := state.Sections[arbiter.PhaseScopeAssumptions].Content

	if features == "" || scope == "" {
		return nil
	}

	var conflicts []arbiter.Conflict

	nonGoalPatterns := []struct {
		nonGoal string
		feature *regexp.Regexp
	}{
		{"no AI", aiPattern},
		{"no mobile", mobileAppPattern},
		{"no social", socialPattern},
	}

	for _, ng := range nonGoalPatterns {
		isNonGoal := strings.Contains(strings.ToLower(scope), strings.ToLower(ng.nonGoal))
		hasFeature := ng.feature.MatchString(features)

		if isNonGoal && hasFeature {
			conflicts = append(conflicts, arbiter.Conflict{
				Type:     arbiter.ConflictScopeCreep,
				Severity: arbiter.SeverityBlocker,
				Message:  "Feature contradicts non-goal: '" + ng.nonGoal + "'",
				Sections: []arbiter.Phase{arbiter.PhaseFeaturesGoals, arbiter.PhaseScopeAssumptions},
			})
		}
	}

	return conflicts
}

// AssumptionChecker detects assumption conflicts
type AssumptionChecker struct{}

func (c *AssumptionChecker) Name() string { return "assumption" }

func (c *AssumptionChecker) Check(state *arbiter.SprintState) []arbiter.Conflict {
	scope := state.Sections[arbiter.PhaseScopeAssumptions].Content
	features := state.Sections[arbiter.PhaseFeaturesGoals].Content

	if scope == "" {
		return nil
	}

	var conflicts []arbiter.Conflict

	assumptionPatterns := []struct {
		assumption string
		requires   *regexp.Regexp
	}{
		{"users have accounts", accountFeature},
		{"users are authenticated", authFeature},
		{"internet connection", offlineFeature},
	}

	for _, ap := range assumptionPatterns {
		hasAssumption := strings.Contains(strings.ToLower(scope), strings.ToLower(ap.assumption))
		hasRequiredFeature := ap.requires.MatchString(features)

		if hasAssumption && !hasRequiredFeature && features != "" {
			conflicts = append(conflicts, arbiter.Conflict{
				Type:     arbiter.ConflictAssumption,
				Severity: arbiter.SeverityWarning,
				Message:  "Assumes '" + ap.assumption + "' but no supporting feature found",
				Sections: []arbiter.Phase{arbiter.PhaseScopeAssumptions, arbiter.PhaseFeaturesGoals},
			})
		}
	}

	return conflicts
}
