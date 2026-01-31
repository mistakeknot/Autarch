# Open Questions via Chat Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `[none] (no bead provided)`

**Goal:** Let users answer scan open questions via the chat panel; the agent resolves them into “Resolved Questions” and the doc pane updates per step.

**Architecture:** Add an agent-layer resolver that accepts dynamic scan context + user answers and returns resolved/remaining questions. Wire a new chat action in Kickoff to invoke it via UnifiedApp, update PhaseArtifacts, and preserve resolved questions across rescans. Use low-stakes auto-apply with easy rollback.

**Tech Stack:** Go, Bubble Tea, Autarch TUI, Autarch agent CLI wrappers.

**Skills:** @agent-native-architecture (context injection + product design), @superpowers:test-driven-development

---

### Task 1: Agent resolver for open questions

**Files:**
- Create: `internal/autarch/agent/open_questions.go`
- Create: `internal/autarch/agent/open_questions_test.go`

**Step 1: Write the failing test**

```go
func TestParseOpenQuestionsResponse(t *testing.T) {
	content := `{"resolved":[{"question":"Q1?","answer":"A1"}],"remaining":["Q2?"]}`
	res, err := parseOpenQuestionsResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Resolved) != 1 || res.Resolved[0].Question != "Q1?" {
		t.Fatalf("unexpected resolved: %#v", res.Resolved)
	}
	if len(res.Remaining) != 1 || res.Remaining[0] != "Q2?" {
		t.Fatalf("unexpected remaining: %#v", res.Remaining)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/autarch/agent -run TestParseOpenQuestionsResponse -v`

Expected: FAIL (undefined parseOpenQuestionsResponse / types)

**Step 3: Write minimal implementation**

```go
type ResolvedQuestion struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type OpenQuestionsResolution struct {
	Resolved  []ResolvedQuestion `json:"resolved"`
	Remaining []string           `json:"remaining"`
}

func ResolveOpenQuestionsWithOutput(ctx context.Context, agent *Agent, input ResolveOpenQuestionsInput, onOutput OutputCallback) (*OpenQuestionsResolution, error) {
	prompt := buildOpenQuestionsPrompt(input)
	resp, err := agent.GenerateWithOutput(ctx, GenerateRequest{Prompt: prompt}, onOutput)
	if err != nil {
		return nil, fmt.Errorf("agent generation failed: %w", err)
	}
	return parseOpenQuestionsResponse(resp.Content)
}
```

Prompt requirements (context injection):
- Include phase label, current summary, evidence list, open questions, and user response.
- Include “known context” (vision/problem/users/platform/language) so the agent sees runtime state.
- Output JSON only with resolved/remaining.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/autarch/agent -run TestParseOpenQuestionsResponse -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/autarch/agent/open_questions.go internal/autarch/agent/open_questions_test.go
git commit -m "feat(agent): resolve open questions from chat"
```

---

### Task 2: TUI types + doc rendering for Resolved Questions

**Files:**
- Modify: `internal/tui/messages.go`
- Modify: `internal/tui/views/kickoff.go`
- Modify: `internal/tui/views/kickoff_doc_test.go`

**Step 1: Write the failing test**

```go
func TestKickoffDocPanelShowsResolvedQuestions(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 30)

	_, _ = v.Update(tui.CodebaseScanResultMsg{
		PhaseArtifacts: &tui.PhaseArtifacts{
			Vision: &tui.VisionArtifact{
				Summary: "Vision text",
				ResolvedQuestions: []tui.ResolvedQuestion{{
					Question: "What is the goal?",
					Answer:   "Ship an agent suite.",
				}},
			},
		},
	})

	view := v.docPanel.View()
	if !strings.Contains(view, "Resolved Questions") {
		t.Fatalf("expected resolved questions section")
	}
	if !strings.Contains(view, "Ship an agent suite") {
		t.Fatalf("expected resolved question answer")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffDocPanelShowsResolvedQuestions -v`

Expected: FAIL (missing ResolvedQuestions)

**Step 3: Write minimal implementation**

- Add `ResolvedQuestion` struct to `internal/tui/messages.go`.
- Add `ResolvedQuestions []ResolvedQuestion` to `VisionArtifact`, `ProblemArtifact`, `UsersArtifact`.
- Update `scanArtifactSummary` + `phaseArtifactForStep` in `internal/tui/views/kickoff.go`.
- Add a “Resolved Questions” section in `addScanEvidenceSections()` that renders Q/A pairs.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffDocPanelShowsResolvedQuestions -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/messages.go internal/tui/views/kickoff.go internal/tui/views/kickoff_doc_test.go
git commit -m "feat(tui): render resolved scan questions"
```

---

### Task 3: Chat → agent wiring for open questions

**Files:**
- Modify: `internal/tui/messages.go`
- Modify: `internal/tui/views/kickoff.go`
- Modify: `internal/tui/unified_app.go`
- Modify: `cmd/autarch/main.go`
- Modify: `cmd/testui/main.go`
- Modify: `internal/tui/views/kickoff_chat_test.go`

**Step 1: Write the failing test**

```go
func TestKickoffEnterSendsOpenQuestionAnswer(t *testing.T) {
	v := NewKickoffView()
	v.scanReview = true
	v.scanResult = &tui.CodebaseScanResultMsg{
		PhaseArtifacts: &tui.PhaseArtifacts{
			Vision: &tui.VisionArtifact{OpenQuestions: []string{"Q1?"}},
		},
	}
	v.SetScanStepForTest(tui.OnboardingScanVision)
	v.chatPanel.SetValue("Answer text")

	called := false
	v.SetResolveOpenQuestionsCallback(func(req tui.OpenQuestionsRequest) tea.Cmd {
		called = true
		return nil
	})

	_, _ = v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !called {
		t.Fatalf("expected resolve callback to fire")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffEnterSendsOpenQuestionAnswer -v`

Expected: FAIL (no callback / Enter handling)

**Step 3: Write minimal implementation**

- Add `OpenQuestionsRequest` + `OpenQuestionsResolvedMsg` types in `internal/tui/messages.go`.
- In `internal/tui/views/kickoff.go`:
  - Add `onResolveOpenQuestions` callback + setter.
  - Intercept `Enter` when `scanReview` is true and there are open questions; add user message to chat, clear composer, call callback.
  - Handle `OpenQuestionsResolvedMsg` by updating current phase artifact: move resolved to `ResolvedQuestions`, set `OpenQuestions` to remaining, then `updateDocPanel()`.
  - Handle `GenerationErrorMsg` when `What == "open-questions"` by adding a system message.
- In `internal/tui/unified_app.go`:
  - Add `resolveOpenQuestionsWithAgent` that calls `agent.ResolveOpenQuestionsWithOutput` with dynamic context (phase, summary, evidence, open questions, user answer, other known scan fields).
  - Extend `agentStreamEvent`/`waitForAgentStream` to return `OpenQuestionsResolvedMsg` when resolution completes.
- In `cmd/autarch/main.go` and `cmd/testui/main.go`, wire the new callback to return a `tea.Msg` that triggers the resolver.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffEnterSendsOpenQuestionAnswer -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/messages.go internal/tui/views/kickoff.go internal/tui/unified_app.go cmd/autarch/main.go cmd/testui/main.go internal/tui/views/kickoff_chat_test.go
git commit -m "feat(tui): answer open questions via chat"
```

---

### Task 4: Preserve resolved questions across rescans

**Files:**
- Modify: `internal/tui/views/kickoff.go`
- Modify: `internal/tui/views/kickoff_doc_test.go`

**Step 1: Write the failing test**

```go
func TestKickoffRescanKeepsResolvedQuestions(t *testing.T) {
	v := NewKickoffView()
	v.scanResult = &tui.CodebaseScanResultMsg{
		PhaseArtifacts: &tui.PhaseArtifacts{
			Vision: &tui.VisionArtifact{
				ResolvedQuestions: []tui.ResolvedQuestion{{Question: "Q1?", Answer: "A1"}},
			},
		},
	}

	msg := tui.CodebaseScanResultMsg{PhaseArtifacts: &tui.PhaseArtifacts{Vision: &tui.VisionArtifact{OpenQuestions: []string{"Q1?", "Q2?"}}}}
	updated := v.applyAcceptedToScanResult(&msg)
	if len(updated.PhaseArtifacts.Vision.ResolvedQuestions) == 0 {
		t.Fatalf("expected resolved questions to carry over")
	}
	for _, q := range updated.PhaseArtifacts.Vision.OpenQuestions {
		if q == "Q1?" {
			t.Fatalf("expected resolved question removed from open list")
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffRescanKeepsResolvedQuestions -v`

Expected: FAIL

**Step 3: Write minimal implementation**

- In `applyAcceptedToScanResult`, merge prior `scanResult` resolved questions into the new scan result for each phase.
- Remove any open questions that match resolved questions (dedupe by question text).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffRescanKeepsResolvedQuestions -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/views/kickoff.go internal/tui/views/kickoff_doc_test.go
git commit -m "fix(tui): keep resolved questions across rescans"
```

---

### Task 5: Basic revert for open question edits (low-stakes approval)

**Files:**
- Modify: `internal/tui/views/kickoff.go`

**Step 1: Write the failing test**

```go
func TestKickoffRevertRestoresSnapshot(t *testing.T) {
	v := NewKickoffView()
	v.scanResult = &tui.CodebaseScanResultMsg{Vision: "Old"}
	snapLabel, snap := v.DocumentSnapshot()
	if snapLabel == "" || snap == "" {
		t.Fatalf("expected snapshot")
	}

	v.scanResult.Vision = "New"
	_, _ = v.Update(tui.RevertLastRunMsg{Snapshot: snap})
	if v.scanResult.Vision != "Old" {
		t.Fatalf("expected revert")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffRevertRestoresSnapshot -v`

Expected: FAIL

**Step 3: Write minimal implementation**

- Implement `DocumentSnapshot()` on `KickoffView` that returns JSON snapshot of scan result + phase artifacts.
- Handle `RevertLastRunMsg` in `KickoffView.Update` by restoring from snapshot JSON and calling `updateDocPanel()`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffRevertRestoresSnapshot -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/views/kickoff.go internal/tui/views/kickoff_doc_test.go
git commit -m "feat(tui): allow revert of open question edits"
```

---

## Notes

- Use “Resolved Questions” (option A) as the display label.
- Keep Enter for chat send during scan review; Ctrl+J remains newline.
- Keep auto-apply since this is low-stakes + reversible (product design guidance).
- Dynamic context injection: include phase, current summary, evidence, and known scan fields in the resolver prompt.

