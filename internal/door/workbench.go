package door

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
	"github.com/mistakeknot/autarch/pkg/review"
)

type workbenchMode int

const (
	workbenchBrowse workbenchMode = iota
	workbenchEditDraft
	workbenchEditLabel
	workbenchEditOutcome
	workbenchConfirmSend
	workbenchConfirmInterrupt
	workbenchSending
	workbenchEditReport
	workbenchEditVerification
	workbenchConfirmVerdict
	workbenchRecording
)

type handoffCaller interface {
	Call(context.Context, review.Request) (review.Response, error)
}

type controllerClient struct{}

func (controllerClient) Call(ctx context.Context, request review.Request) (review.Response, error) {
	client, err := review.EnsureController(ctx)
	if err != nil {
		return review.Response{}, err
	}
	return client.Call(ctx, request)
}

type CapacityObservation struct {
	State      string
	ObservedAt time.Time
}

type RecommendationContext struct {
	RequiredProvider string
	ContinuityTarget agenttransport.Target
	Readiness        map[string]string
	Capacity         map[string]CapacityObservation
	PreferenceTarget agenttransport.Target
}

func providerIdentity(target agenttransport.Target) string {
	if target.Provider() == "codex" {
		return "Codex"
	}
	if target.Provider() == "claude-code" {
		return "Claude Code"
	}
	return "unsupported"
}

func agentPane(pane agenttransport.Pane) bool {
	return !pane.Dead && providerIdentity(pane.Target) != "unsupported"
}

// RecommendPanes applies policy, task continuity, observed readiness/capacity,
// then the exact saved preference. Unknown capacity never becomes availability.
func RecommendPanes(panes []agenttransport.Pane, context RecommendationContext) []agenttransport.Pane {
	out := make([]agenttransport.Pane, 0, len(panes))
	for _, pane := range panes {
		if agentPane(pane) {
			out = append(out, pane.Clone())
		}
	}
	score := func(pane agenttransport.Pane) [4]int {
		key := pane.Target.Key()
		policy := 0
		if context.RequiredProvider == "" || providerIdentity(pane.Target) == context.RequiredProvider {
			policy = 1
		}
		continuity := 0
		if context.ContinuityTarget.Socket != "" && pane.Target.SamePane(context.ContinuityTarget) {
			continuity = 1
		}
		observed := 0
		if context.Readiness[key] == "ready" {
			observed += 2
		}
		if capacity := context.Capacity[key]; capacity.State == "available" && !capacity.ObservedAt.IsZero() {
			observed++
		}
		preference := 0
		if context.PreferenceTarget.Socket != "" && pane.Target == context.PreferenceTarget {
			preference = 1
		}
		return [4]int{policy, continuity, observed, preference}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := score(out[i]), score(out[j])
		for n := range left {
			if left[n] != right[n] {
				return left[n] > right[n]
			}
		}
		return out[i].Target.Key() < out[j].Target.Key()
	})
	return out
}

type workbenchState struct {
	transport agenttransport.Transport
	client    handoffCaller
	adapter   WorkAdapter

	preferencePath string
	preference     WorkbenchPreference
	panes          []agenttransport.Pane
	target         agenttransport.Target
	targetStale    bool
	preview        string
	previewError   string
	readiness      string
	capacity       CapacityObservation
	accountLabel   string

	outcome   string
	task      review.TaskIdentity
	taskTitle string
	discovery WorkDiscovery
	kind      string
	draft     string
	mode      workbenchMode
	followup  bool

	pendingRequestID  string
	pendingHandoff    review.ExternalHandoff
	lastHandoffID     string
	lastTarget        agenttransport.Target
	lastTask          review.TaskIdentity
	lastMessage       string
	lastDelivery      agenttransport.Delivery
	openRequired      bool
	recoveryHandoffID string
	recoveryTarget    agenttransport.Target
	handoffs          []review.ExternalHandoff
	recordInput       string
	pendingVerdict    string
}

func newWorkbenchState(project string) workbenchState {
	path := DefaultPreferencePath()
	preference, preferenceErr := LoadWorkbenchPreference(path, project)
	state := workbenchState{
		transport:      agenttransport.NewTmux(nil, ""),
		client:         controllerClient{},
		adapter:        NewWorkAdapter(WorkAdapterConfigFromEnv()),
		preferencePath: path,
		preference:     preference,
		readiness:      "uncertain",
		kind:           "investigation",
		discovery:      WorkDiscovery{State: "unavailable", Error: "clavain work adapter not configured; reviewed candidate is not qualified or installed"},
	}
	state.outcome = preference.Outcome
	state.task = preference.Task
	state.target = preference.Target
	state.targetStale = preference.Target.Socket != ""
	state.accountLabel = preference.AccountLabel
	state.draft = preference.Draft
	state.recoveryHandoffID = preference.RecoveryHandoffID
	state.recoveryTarget = preference.RecoveryTarget
	state.openRequired = preference.RecoveryHandoffID != ""
	if preferenceErr != nil {
		state.previewError = "preferences unreadable: " + preferenceErr.Error()
	}
	return state
}

func (m *Model) restoreWorkbenchPreference(preference WorkbenchPreference) {
	m.workbench.preference = preference
	m.workbench.outcome = preference.Outcome
	m.workbench.task = preference.Task
	m.workbench.target = preference.Target
	m.workbench.accountLabel = preference.AccountLabel
	m.workbench.draft = preference.Draft
	m.workbench.recoveryHandoffID = preference.RecoveryHandoffID
	m.workbench.recoveryTarget = preference.RecoveryTarget
	m.workbench.openRequired = preference.RecoveryHandoffID != ""
	m.workbench.targetStale = preference.Target.Socket != ""
}

func (m *Model) workbenchPreference() WorkbenchPreference {
	return WorkbenchPreference{Outcome: m.workbench.outcome, Task: m.workbench.task, Target: m.workbench.target, AccountLabel: m.workbench.accountLabel, Draft: m.workbench.draft, RecoveryHandoffID: m.workbench.recoveryHandoffID, RecoveryTarget: m.workbench.recoveryTarget}
}

func (m *Model) saveWorkbenchPreference() {
	if err := SaveWorkbenchPreference(m.workbench.preferencePath, m.productRoot, m.workbenchPreference()); err != nil {
		m.status = "Preferences not saved: " + err.Error()
	}
}

type workbenchLoadedMsg struct {
	panes     []agenttransport.Pane
	discovery WorkDiscovery
	err       error
}

type workbenchPreviewMsg struct {
	target  agenttransport.Target
	preview string
	err     error
}

type workbenchPreviewTick struct{}

func nextWorkbenchPreviewTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return workbenchPreviewTick{} })
}

type workbenchActionMsg struct {
	action   string
	id       string
	delivery agenttransport.Delivery
	preview  string
	err      error
}

type workbenchRecordMsg struct {
	method  string
	id      string
	handoff review.ExternalHandoff
	err     error
}

func (m Model) loadWorkbench() tea.Cmd {
	transport, adapter, root, task := m.workbench.transport, m.workbench.adapter, m.productRoot, m.workbench.task
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		panes, err := transport.List(ctx)
		evidence := root
		if task.Qualification == "qualified" && task.TrackerUUID != "" && task.BeadID != "" {
			evidence += " " + task.TrackerUUID + ":" + task.BeadID
		}
		return workbenchLoadedMsg{panes: panes, discovery: adapter.Read(ctx, evidence), err: err}
	}
}

func (m Model) loadWorkbenchPreview() tea.Cmd {
	transport, target := m.workbench.transport, m.workbench.target
	if target.Socket == "" || m.workbench.targetStale {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		preview, err := transport.Observe(ctx, target)
		return workbenchPreviewMsg{target: target, preview: preview, err: err}
	}
}

func (m *Model) applyWorkbenchPanes(panes []agenttransport.Pane) {
	preference := m.workbench.target
	ranked := RecommendPanes(panes, RecommendationContext{PreferenceTarget: preference, ContinuityTarget: m.workbench.lastTarget})
	m.workbench.panes = ranked
	m.workbench.targetStale = preference.Socket != ""
	for _, pane := range ranked {
		if pane.Target == preference {
			m.workbench.targetStale = false
			return
		}
	}
	if preference.Socket == "" && len(ranked) > 0 {
		m.workbench.target = ranked[0].Target
		m.workbench.targetStale = false
		m.saveWorkbenchPreference()
	}
}

func (m *Model) adoptProductForWorkbench() {
	if m.workbench.outcome == "" {
		m.workbench.outcome = m.product.Card.Fields["cuj"].Value
	}
	choices := m.workbenchTaskChoices()
	for _, choice := range choices {
		if choice.identity == m.workbench.task {
			m.workbench.taskTitle = choice.title
			m.saveWorkbenchPreference()
			return
		}
	}
	if m.workbench.task.Tracker != "" || m.workbench.task.TrackerUUID != "" {
		m.workbench.taskTitle = "saved task unavailable in the current read"
	} else if len(choices) > 0 {
		m.workbench.task, m.workbench.taskTitle = choices[0].identity, choices[0].title
	}
	m.saveWorkbenchPreference()
}

type workbenchTaskChoice struct {
	identity review.TaskIdentity
	title    string
}

func (m Model) workbenchTaskChoices() []workbenchTaskChoice {
	var choices []workbenchTaskChoice
	if m.workbench.discovery.State == "read" {
		for _, task := range m.workbench.discovery.Tasks {
			choices = append(choices, workbenchTaskChoice{identity: review.TaskIdentity{Tracker: m.workbench.adapter.config.Registry, TrackerUUID: task.TrackerUUID, BeadID: task.BeadID, Assignee: task.Assignee, Qualification: "qualified"}, title: task.Title})
		}
		return choices
	}
	for _, work := range m.product.Backlog.Items {
		choices = append(choices, workbenchTaskChoice{identity: review.TaskIdentity{Tracker: productScope(m.product.Backlog), BeadID: work.ID, ID: work.ID, Assignee: displayCandidateAssignee(work.Assignee), Qualification: "unqualified"}, title: work.Title})
	}
	return choices
}

func (m *Model) cycleWorkbenchTask() {
	choices := m.workbenchTaskChoices()
	if len(choices) == 0 {
		m.status = "No current task candidates; the saved task remains unchanged"
		return
	}
	index := -1
	for i, choice := range choices {
		if choice.identity == m.workbench.task {
			index = i
			break
		}
	}
	next := choices[(index+1)%len(choices)]
	m.workbench.task, m.workbench.taskTitle = next.identity, next.title
	m.workbench.followup = false
	m.saveWorkbenchPreference()
}

func (m Model) currentPane() (agenttransport.Pane, bool) {
	for _, pane := range m.workbench.panes {
		if pane.Target == m.workbench.target {
			return pane, true
		}
	}
	return agenttransport.Pane{}, false
}

func (m Model) workbenchContext() review.HandoffContext {
	context := review.HandoffContext{Outcome: m.workbench.outcome, Scope: firstText(m.workbench.taskTitle, m.workbench.outcome)}
	if m.product.Card.Status == "confirmed" {
		guardrail := m.product.Card.Fields["guardrail"]
		if guardrail.State == "confirmed" && strings.TrimSpace(guardrail.Value) != "" {
			source := "docs/why.md#fields.guardrail"
			for _, evidence := range guardrail.Evidence {
				if evidence.Scope == "project" && evidence.Path != "" {
					source = evidence.Path
					break
				}
			}
			context.Decisions = append(context.Decisions, review.HandoffDecision{Literal: strings.TrimSpace(guardrail.Value), Source: source})
		}
	}
	primary := m.product.Card.Fields["cuj"]
	for _, journey := range m.product.Journeys {
		if journey.ID == primary.Ref || journey.Source.Path == primary.Path {
			if journey.Success != "" {
				context.AcceptanceChecks = append(context.AcceptanceChecks, journey.Success)
			} else if recognition := markdownRecognition(journey.Source.Content); recognition != "" {
				context.AcceptanceChecks = append(context.AcceptanceChecks, recognition)
			}
		}
	}
	if len(context.AcceptanceChecks) == 0 {
		if success := m.product.Card.Fields["success"].Value; success != "" {
			context.AcceptanceChecks = append(context.AcceptanceChecks, success)
		}
	}
	context.ImplementationAuthorized = m.workbench.kind == "implementation"
	return context
}

func markdownRecognition(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var parts []string
	reading := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !reading {
			if after, ok := strings.CutPrefix(trimmed, "Recognition condition:"); ok {
				reading = true
				if strings.TrimSpace(after) != "" {
					parts = append(parts, strings.TrimSpace(after))
				}
			}
			continue
		}
		if trimmed == "" {
			break
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

func (m Model) buildPendingHandoff(interrupt bool) (review.ExternalHandoff, error) {
	if strings.TrimSpace(m.workbench.draft) == "" {
		return review.ExternalHandoff{}, errors.New("write a direction first")
	}
	if m.workbench.targetStale || m.workbench.target.Socket == "" {
		return review.ExternalHandoff{}, errors.New("select a current exact pane")
	}
	if m.workbench.openRequired {
		return review.ExternalHandoff{}, errors.New("open the original pane before approving more input")
	}
	if m.workbench.task.Assignee == "" {
		return review.ExternalHandoff{}, errors.New("task has no recorded assignee")
	}
	if m.workbench.task.Qualification != "qualified" && m.workbench.task.Qualification != "unqualified" {
		return review.ExternalHandoff{}, errors.New("task qualification is not explicit")
	}
	if err := m.workbench.target.Validate(); err != nil {
		return review.ExternalHandoff{}, err
	}
	form, parent := "full", ""
	if m.workbench.followup && m.workbench.lastHandoffID != "" && m.workbench.target == m.workbench.lastTarget && m.workbench.task == m.workbench.lastTask {
		form, parent = "delta", m.workbench.lastHandoffID
	}
	handoff := review.ExternalHandoff{Task: m.workbench.task, Target: m.workbench.target, Draft: m.workbench.draft, Kind: m.workbench.kind, Form: form, ParentID: parent, Readiness: m.workbench.readiness, Context: m.workbenchContext()}
	if handoff.Form == "full" && (handoff.Context.Outcome == "" || handoff.Context.Scope == "" || len(handoff.Context.AcceptanceChecks) == 0) {
		return review.ExternalHandoff{}, errors.New("full handoff needs a selected outcome, scope, and source-backed acceptance check")
	}
	if interrupt {
		handoff.Form, handoff.ParentID = "full", ""
	}
	handoff.Message = handoff.RenderedMessage()
	if len(handoff.Message) > 1<<20 || strings.ContainsAny(handoff.Message, "\x00\x1b\r") {
		return review.ExternalHandoff{}, errors.New("handoff message is too large or contains terminal control bytes")
	}
	if !interrupt && !m.workbench.followup && m.workbench.lastDelivery == agenttransport.Delivered && handoff.Target == m.workbench.lastTarget && handoff.Task == m.workbench.lastTask && handoff.Message == m.workbench.lastMessage {
		return review.ExternalHandoff{}, errors.New("this exact message was already approved; edit it or choose Follow up")
	}
	return handoff, nil
}

func (m Model) runHandoff(action string) tea.Cmd {
	client, transport := m.workbench.client, m.workbench.transport
	requestID, project, handoff := m.workbench.pendingRequestID, m.productRoot, m.workbench.pendingHandoff.Clone()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		approved, err := client.Call(ctx, review.Request{Version: review.Version, ID: requestID, Method: "handoff.approve", Project: project, Actor: "human", Handoff: &handoff})
		if err != nil {
			return workbenchActionMsg{action: action, err: err}
		}
		method := "handoff.deliver"
		if action == "interrupt" {
			method = "handoff.interrupt"
		}
		result, err := client.Call(ctx, review.Request{Version: review.Version, ID: review.NewID(), Method: method, Project: project, Target: approved.ID})
		msg := workbenchActionMsg{action: action, id: approved.ID, delivery: result.Delivery, err: err}
		if action == "interrupt" {
			msg.preview, _ = transport.Observe(ctx, handoff.Target)
		}
		return msg
	}
}

func (m Model) openWorkbenchPane() tea.Cmd {
	transport, client, target := m.workbench.transport, m.workbench.client, m.workbench.target
	id, project, recorded := m.workbench.recoveryHandoffID, m.productRoot, m.workbench.openRequired
	if recorded {
		target = m.workbench.recoveryTarget
	}
	recordOpen := func(ctx context.Context) error {
		if !recorded || id == "" {
			return nil
		}
		_, err := client.Call(ctx, review.Request{Version: review.Version, ID: review.NewID(), Method: "handoff.opened", Project: project, Target: id, Actor: "human", Handoff: &review.ExternalHandoff{Target: target}})
		return err
	}
	if os.Getenv("TMUX") == "" {
		if tmux, ok := transport.(*agenttransport.Tmux); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			command, err := tmux.AttachCommand(ctx, target)
			cancel()
			if err != nil {
				return func() tea.Msg { return workbenchActionMsg{action: "open", id: id, err: err} }
			}
			return tea.ExecProcess(command, func(err error) tea.Msg {
				if err == nil {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					err = recordOpen(ctx)
				}
				return workbenchActionMsg{action: "open", id: id, err: err}
			})
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := transport.Open(ctx, target)
		if err == nil {
			err = recordOpen(ctx)
		}
		return workbenchActionMsg{action: "open", id: id, err: err}
	}
}

type handoffReportInput struct {
	Completion    string   `json:"completion"`
	Commit        string   `json:"commit"`
	Files         []string `json:"files"`
	Checks        []string `json:"checks"`
	RunnableBuild string   `json:"runnable_build"`
}

func decodeWorkbenchJSON(input string, value any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("record contains trailing or malformed JSON")
	}
	return nil
}

func (m Model) latestWorkbenchHandoff() (review.ExternalHandoff, bool) {
	if len(m.workbench.handoffs) == 0 {
		return review.ExternalHandoff{}, false
	}
	return m.workbench.handoffs[len(m.workbench.handoffs)-1].Clone(), true
}

func (m *Model) selectWorkbenchRecovery() {
	localID, localTarget := m.workbench.recoveryHandoffID, m.workbench.recoveryTarget
	m.workbench.openRequired = false
	m.workbench.recoveryHandoffID = ""
	m.workbench.recoveryTarget = agenttransport.Target{}
	for _, handoff := range m.workbench.handoffs {
		if handoff.OpenRequired {
			m.workbench.openRequired = true
			m.workbench.recoveryHandoffID = handoff.ID
			m.workbench.recoveryTarget = handoff.Target
			return
		}
	}
	if localID != "" {
		m.workbench.openRequired = true
		m.workbench.recoveryHandoffID = localID
		m.workbench.recoveryTarget = localTarget
	}
}

func (m Model) runWorkbenchRecord(method string, handoff review.ExternalHandoff) tea.Cmd {
	client, project := m.workbench.client, m.productRoot
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		request := review.Request{Version: review.Version, ID: review.NewID(), Method: method, Project: project, Target: handoff.ID, Handoff: &handoff}
		if method == "handoff.verify" || method == "handoff.verdict" {
			request.Actor = "human"
		}
		_, err := client.Call(ctx, request)
		return workbenchRecordMsg{method: method, id: handoff.ID, handoff: handoff, err: err}
	}
}

func (m Model) finishWorkbenchRecord() (tea.Model, tea.Cmd) {
	handoff, ok := m.latestWorkbenchHandoff()
	if !ok {
		m.workbench.mode = workbenchBrowse
		m.status = "No external handoff result is selected"
		return m, nil
	}
	switch m.workbench.mode {
	case workbenchEditReport:
		var input handoffReportInput
		if err := decodeWorkbenchJSON(m.workbench.recordInput, &input); err != nil {
			m.status = "Agent report JSON invalid: " + err.Error()
			return m, nil
		}
		if strings.TrimSpace(input.Completion) == "" {
			m.status = "Agent report needs a completion statement"
			return m, nil
		}
		handoff.AgentReportedCompletion = input.Completion
		handoff.Results = review.HandoffResults{Commit: input.Commit, Files: input.Files, Checks: input.Checks, RunnableBuild: input.RunnableBuild}
		m.workbench.mode = workbenchRecording
		return m, m.runWorkbenchRecord("handoff.report", handoff)
	case workbenchEditVerification:
		var check review.HandoffCheck
		if err := decodeWorkbenchJSON(m.workbench.recordInput, &check); err != nil {
			m.status = "Verification JSON invalid: " + err.Error()
			return m, nil
		}
		handoff.VerifiedChecks = []review.HandoffCheck{check}
		m.workbench.mode = workbenchRecording
		return m, m.runWorkbenchRecord("handoff.verify", handoff)
	}
	return m, nil
}

func (m Model) workbenchVerdictLines() []string {
	handoff, ok := m.latestWorkbenchHandoff()
	if !ok {
		return []string{"No external handoff result is selected."}
	}
	return []string{
		"CONFIRM HUMAN VERDICT · " + strings.ToUpper(m.workbench.pendingVerdict),
		"External handoff: " + handoff.ID,
		"Agent report: " + firstText(handoff.AgentReportedCompletion, "none"),
		fmt.Sprintf("Verified checks attached: %d", len(handoff.VerifiedChecks)),
		"Evidence: " + verdictEvidence(handoff),
		"y Record verdict · Esc Cancel",
	}
}

func verdictEvidence(handoff review.ExternalHandoff) string {
	return fmt.Sprintf("human reviewed exact handoff %s with %d verified check(s); commit %s; runnable build %s", handoff.ID, len(handoff.VerifiedChecks), firstText(handoff.Results.Commit, "unknown"), firstText(handoff.Results.RunnableBuild, "unknown"))
}

func appendText(value string, msg tea.KeyMsg, multiline bool) (string, bool) {
	switch msg.String() {
	case "backspace":
		runes := []rune(value)
		if len(runes) > 0 {
			value = string(runes[:len(runes)-1])
		}
		return value, true
	case "enter":
		if multiline {
			return value + "\n", true
		}
		return value, false
	case "space":
		return value + " ", true
	}
	if msg.Type == tea.KeyRunes {
		return value + string(msg.Runes), true
	}
	return value, true
}

func (m Model) handleWorkbenchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch m.workbench.mode {
	case workbenchEditDraft, workbenchEditLabel, workbenchEditOutcome, workbenchEditReport, workbenchEditVerification:
		if key == "esc" {
			m.workbench.mode = workbenchBrowse
			m.workbench.recordInput = ""
			m.saveWorkbenchPreference()
			return m, nil
		}
		if key == "ctrl+s" {
			if m.workbench.mode == workbenchEditReport || m.workbench.mode == workbenchEditVerification {
				return m.finishWorkbenchRecord()
			}
			m.workbench.mode = workbenchBrowse
			m.saveWorkbenchPreference()
			return m, nil
		}
		var changed bool
		switch m.workbench.mode {
		case workbenchEditDraft:
			m.workbench.draft, changed = appendText(m.workbench.draft, msg, true)
		case workbenchEditLabel:
			m.workbench.accountLabel, changed = appendText(m.workbench.accountLabel, msg, false)
		case workbenchEditOutcome:
			m.workbench.outcome, changed = appendText(m.workbench.outcome, msg, false)
		case workbenchEditReport, workbenchEditVerification:
			previous := m.workbench.recordInput
			m.workbench.recordInput, changed = appendText(m.workbench.recordInput, msg, true)
			if len(m.workbench.recordInput) > 1<<20 {
				m.workbench.recordInput = previous
				m.status = "Result record is limited to 1 MiB"
			}
		}
		if !changed {
			m.workbench.mode = workbenchBrowse
		}
		if m.workbench.mode != workbenchEditReport && m.workbench.mode != workbenchEditVerification {
			m.saveWorkbenchPreference()
		}
		return m, nil
	case workbenchConfirmSend, workbenchConfirmInterrupt:
		if key == "esc" || key == "n" {
			m.workbench.mode = workbenchBrowse
			m.workbench.pendingHandoff = review.ExternalHandoff{}
			m.workbench.pendingRequestID = ""
			return m, nil
		}
		if key == "y" {
			action := "send"
			if m.workbench.mode == workbenchConfirmInterrupt {
				action = "interrupt"
			}
			m.workbench.mode = workbenchSending
			return m, m.runHandoff(action)
		}
		return m, nil
	case workbenchConfirmVerdict:
		if key == "esc" || key == "n" {
			m.workbench.mode = workbenchBrowse
			m.workbench.pendingVerdict = ""
			return m, nil
		}
		if key == "y" {
			handoff, ok := m.latestWorkbenchHandoff()
			if !ok {
				m.workbench.mode = workbenchBrowse
				m.status = "No external handoff result is selected"
				return m, nil
			}
			handoff.HumanVerdict = &review.HandoffVerdict{Result: m.workbench.pendingVerdict, Evidence: verdictEvidence(handoff)}
			m.workbench.mode = workbenchRecording
			return m, m.runWorkbenchRecord("handoff.verdict", handoff)
		}
		return m, nil
	case workbenchSending, workbenchRecording:
		return m, nil
	}
	switch key {
	case "e":
		m.workbench.mode = workbenchEditDraft
	case "l":
		m.workbench.mode = workbenchEditLabel
	case "O":
		m.workbench.mode = workbenchEditOutcome
	case "i":
		if m.workbench.kind == "investigation" {
			m.workbench.kind = "implementation"
		} else {
			m.workbench.kind = "investigation"
		}
	case "a":
		if len(m.workbench.panes) > 0 {
			index := -1
			for i, pane := range m.workbench.panes {
				if pane.Target == m.workbench.target {
					index = i
				}
			}
			m.workbench.target = m.workbench.panes[(index+1)%len(m.workbench.panes)].Target
			m.workbench.targetStale = false
			m.workbench.preview, m.workbench.previewError = "", ""
			m.workbench.followup = false
			m.workbench.readiness = "uncertain"
			m.saveWorkbenchPreference()
			return m, m.loadWorkbenchPreview()
		}
	case "m":
		m.cycleWorkbenchTask()
	case "u":
		switch m.workbench.readiness {
		case "uncertain":
			m.workbench.readiness = "ready"
		case "ready":
			m.workbench.readiness = "busy"
		default:
			m.workbench.readiness = "uncertain"
		}
	case "r":
		return m, m.loadWorkbench()
	case "R":
		if _, ok := m.latestWorkbenchHandoff(); !ok {
			m.status = "No external handoff result is selected"
			return m, nil
		}
		m.workbench.recordInput = `{"completion":"","commit":"","files":[],"checks":[],"runnable_build":""}`
		m.workbench.mode = workbenchEditReport
	case "V":
		if _, ok := m.latestWorkbenchHandoff(); !ok {
			m.status = "No external handoff result is selected"
			return m, nil
		}
		m.workbench.recordInput = `{"name":"","result":"inconclusive","evidence":""}`
		m.workbench.mode = workbenchEditVerification
	case "P", "F", "I":
		if _, ok := m.latestWorkbenchHandoff(); !ok {
			m.status = "No external handoff result is selected"
			return m, nil
		}
		m.workbench.pendingVerdict = map[string]string{"P": "pass", "F": "fail", "I": "inconclusive"}[key]
		m.workbench.mode = workbenchConfirmVerdict
		m.productOffset = 0
	case "f":
		if m.workbench.lastHandoffID != "" && m.workbench.target == m.workbench.lastTarget && m.workbench.task == m.workbench.lastTask {
			m.workbench.followup = true
			m.status = "Corrective follow-up will be a delta to the same exact pane"
		} else {
			m.workbench.followup = false
			m.status = "Pane or task changed; the next handoff will include full context"
		}
	case "s", "x":
		interrupt := key == "x"
		if m.workbench.openRequired {
			m.status = "Open the original pane before approving more input"
			return m, nil
		}
		if !interrupt && m.workbench.readiness != "ready" {
			m.status = "Send blocked: readiness must be explicitly marked ready"
			return m, nil
		}
		if interrupt && m.workbench.readiness == "ready" {
			m.status = "Interrupt is only available for a busy or uncertain pane"
			return m, nil
		}
		handoff, err := m.buildPendingHandoff(interrupt)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.workbench.pendingHandoff = handoff
		m.workbench.pendingRequestID = review.NewID()
		if interrupt {
			m.workbench.mode = workbenchConfirmInterrupt
		} else {
			m.workbench.mode = workbenchConfirmSend
		}
		m.productOffset = 0
	case "t":
		if m.workbench.target.Socket == "" {
			m.status = "No exact pane selected"
			return m, nil
		}
		return m, m.openWorkbenchPane()
	}
	return m, nil
}

func targetSummary(target agenttransport.Target) string {
	if target.Socket == "" {
		return "none"
	}
	return fmt.Sprintf("socket %s · server %d/%d · session %s · window %s · pane %s · pid %d", target.Socket, target.ServerPID, target.ServerStarted, target.SessionID, target.WindowID, target.PaneID, target.PanePID)
}

func taskSummary(task review.TaskIdentity) string {
	if task.Qualification == "qualified" {
		return "qualified · tracker " + task.TrackerUUID + " · Bead " + task.BeadID
	}
	return "UNQUALIFIED · " + firstText(task.BeadID, task.ID, "no task selected")
}

func firstText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (m Model) confirmationLines(interrupt bool) []string {
	h := m.workbench.pendingHandoff
	title := "CONFIRM SEND"
	verb := "send"
	if interrupt {
		title, verb = "CONFIRM INTERRUPT AND REDIRECT", "interrupt, wait for provider readiness, then redirect"
	}
	return []string{
		title,
		"No terminal input occurs until y.",
		"Project: " + m.productRoot,
		"Task: " + taskSummary(h.Task),
		"Recorded assignee: " + h.Task.Assignee,
		"Destination: " + targetSummary(h.Target),
		"Provider identity: " + providerIdentity(h.Target),
		"Account label: " + firstText(m.workbench.accountLabel, "not set"),
		"Instruction kind: " + h.Kind,
		"Exact message:", h.Message,
		"y Confirm and " + verb + " this exact pane · Esc Cancel",
	}
}

func (m Model) workbenchLines() []string {
	if m.workbench.mode == workbenchConfirmSend {
		return m.confirmationLines(false)
	}
	if m.workbench.mode == workbenchConfirmInterrupt {
		return m.confirmationLines(true)
	}
	if m.workbench.mode == workbenchConfirmVerdict {
		return m.workbenchVerdictLines()
	}
	task := taskSummary(m.workbench.task)
	if m.workbench.taskTitle != "" {
		task += " · " + m.workbench.taskTitle
	}
	agent := "No current Claude Code or Codex pane selected"
	if m.workbench.target.Socket != "" {
		agent = providerIdentity(m.workbench.target) + " · " + targetSummary(m.workbench.target)
		if m.workbench.targetStale {
			agent += " · STALE (never rebound by name)"
		}
	}
	capacity := "unknown"
	if m.workbench.capacity.State != "" && m.workbench.capacity.State != "unknown" && !m.workbench.capacity.ObservedAt.IsZero() {
		capacity = m.workbench.capacity.State + " · observed " + m.workbench.capacity.ObservedAt.Local().Format("15:04:05")
	}
	preview := oneLine(m.workbench.preview)
	if preview == "" {
		preview = firstText(m.workbench.previewError, "not read")
	}
	if m.dashboardContentWidth() < 55 {
		lines := []string{
			"OUTCOME · " + firstText(m.workbench.outcome, "Not chosen"),
			"CURRENT WORK · " + task,
			"AGENT · " + providerIdentity(m.workbench.target) + " · " + firstText(m.workbench.target.PaneID, "none"),
			"NEXT ACTION · " + firstText(oneLine(m.workbench.draft), "Draft empty"),
			"Readiness " + m.workbench.readiness + " · capacity unknown",
			"e Draft · O Outcome · l Account · i Kind · s Send · x Interrupt",
		}
		switch m.workbench.mode {
		case workbenchEditDraft:
			lines = append(lines, "EDIT DRAFT · Enter newline · Ctrl+S finish")
		case workbenchEditLabel:
			lines = append(lines, "EDIT ACCOUNT LABEL · Enter/Ctrl+S finish")
		case workbenchEditOutcome:
			lines = append(lines, "EDIT OUTCOME · Enter/Ctrl+S finish")
		case workbenchEditReport:
			lines = append(lines, "EDIT AGENT REPORT JSON · Ctrl+S record · Esc cancel", m.workbench.recordInput)
		case workbenchEditVerification:
			lines = append(lines, "EDIT VERIFIED CHECK JSON · Ctrl+S record · Esc cancel", m.workbench.recordInput)
		case workbenchSending:
			lines = append(lines, "Persisting intent and contacting the exact pane…")
		case workbenchRecording:
			lines = append(lines, "Recording result evidence without sending terminal input…")
		}
		return append(lines, m.workbenchResultLines()...)
	}
	lines := []string{
		"OUTCOME", firstText(m.workbench.outcome, "Not chosen"),
		"CURRENT WORK", task, "Recorded assignee: " + firstText(m.workbench.task.Assignee, "unknown"),
		"AGENT", agent, "Account label: " + firstText(m.workbench.accountLabel, "not set") + " · Provider identity: " + providerIdentity(m.workbench.target),
		"Readiness: " + m.workbench.readiness + " · Capacity/allowance: " + capacity,
		"Recent output (local, bounded): " + preview,
		"NEXT ACTION", "Kind: " + m.workbench.kind + " · Draft: " + firstText(oneLine(m.workbench.draft), "empty"),
		fmt.Sprintf("%d alternative task(s) · e Draft · O Outcome · l Account · m Task · a Agent · u Readiness", len(m.workbenchTaskChoices())),
		"s Send · f Follow up · t Switch to terminal · x Interrupt and redirect",
	}
	if m.workbench.discovery.State != "read" {
		lines = append(lines, "Task adapter unavailable · ProductBacklog fallback · "+m.workbench.discovery.Error)
	} else if len(m.workbench.discovery.Ambiguous) > 0 {
		lines = append(lines, fmt.Sprintf("Task adapter: %d ambiguous match(es); selection is not inferred", len(m.workbench.discovery.Ambiguous)))
	}
	if m.workbench.mode == workbenchEditDraft {
		lines = append(lines, "EDIT DRAFT · Enter newline · Ctrl+S finish")
	} else if m.workbench.mode == workbenchEditLabel {
		lines = append(lines, "EDIT ACCOUNT LABEL · Enter/Ctrl+S finish")
	} else if m.workbench.mode == workbenchEditOutcome {
		lines = append(lines, "EDIT OUTCOME · Enter/Ctrl+S finish")
	} else if m.workbench.mode == workbenchEditReport {
		lines = append(lines, "EDIT AGENT REPORT JSON · Ctrl+S record · Esc cancel", m.workbench.recordInput)
	} else if m.workbench.mode == workbenchEditVerification {
		lines = append(lines, "EDIT VERIFIED CHECK JSON · result pass/fail/inconclusive · Ctrl+S record · Esc cancel", m.workbench.recordInput)
	} else if m.workbench.mode == workbenchSending {
		lines = append(lines, "Persisting intent and contacting the exact pane…")
	} else if m.workbench.mode == workbenchRecording {
		lines = append(lines, "Recording result evidence without sending terminal input…")
	}
	if m.workbench.openRequired {
		lines = append(lines, "Open the original pane for "+m.workbench.recoveryHandoffID+"; a fresh explicit approval is required before later input.")
	}
	lines = append(lines, m.workbenchResultLines()...)
	return lines
}

func (m Model) workbenchResultLines() []string {
	if len(m.workbench.handoffs) == 0 {
		return nil
	}
	h := m.workbench.handoffs[len(m.workbench.handoffs)-1]
	lines := []string{"EXTERNAL HANDOFF RESULT", "Agent reported: " + firstText(h.AgentReportedCompletion, "none")}
	if h.Results.Commit != "" {
		lines = append(lines, "Result refs: commit "+h.Results.Commit+" · files "+strings.Join(h.Results.Files, ", "))
	}
	if len(h.Results.Checks) > 0 {
		lines = append(lines, "Agent-reported checks: "+strings.Join(h.Results.Checks, " · "))
	}
	if h.Results.RunnableBuild != "" {
		lines = append(lines, "Runnable build: "+h.Results.RunnableBuild)
	}
	lines = append(lines, "Verified checks:")
	for _, check := range h.VerifiedChecks {
		lines = append(lines, "  "+check.Result+" · "+check.Name+" · "+check.Evidence)
	}
	verdict := "none"
	if h.HumanVerdict != nil {
		verdict = h.HumanVerdict.Result + " · " + h.HumanVerdict.Evidence
	}
	return append(lines, "Human verdict: "+verdict, "R Agent report · V Verified check · P/F/I Human verdict · f Corrective follow-up")
}

func cleanProjectName(root string) string { return filepath.Base(filepath.Clean(root)) }
