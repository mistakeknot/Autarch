package tui

import (
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// GurgehConfig configures the GurgehOnboardingView with onboarding dependencies.
type GurgehConfig struct {
	ResearchCoord *research.Coordinator
	CodingAgent   *agent.Agent
	AgentSelector *pkgtui.AgentSelector
	SelectedAgent string

	// View factories for onboarding steps
	CreateKickoffView     func() View
	CreateArbiterView     func(*research.Coordinator) View
	CreateSpecSummaryView func(*SpecSummary, *research.Coordinator) View
	CreateEpicReviewView  func([]epics.EpicProposal) View
	CreateTaskReviewView  func([]tasks.TaskProposal) View
	CreateTaskDetailView  func(tasks.TaskProposal, *research.Coordinator) View
	CreateSprintView      func(string) View
}
