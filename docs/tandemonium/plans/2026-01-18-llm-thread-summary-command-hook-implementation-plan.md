# LLM Thread Summaries via Command Hook Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `n/a (no bead id provided)` — mandatory line tying the plan to the active bead/Task Master item.

**Goal:** Add LLM-backed thread summaries with examples using a user-configurable command hook (default example: Claude CLI).

**Architecture:** Introduce a new config section for an external summary command. When `summarize_thread` is called with `include_examples` or `llm_mode`, Tandemonium executes the command with JSON input and expects JSON output. Keep metadata-only fallback if command is absent or fails. Store no credentials; defer to user’s CLI agent.

**Tech Stack:** Go, Cobra, standard library (os/exec, encoding/json, context).

### Task 1: Add config + command runner for LLM summaries

**Files:**
- Modify: `internal/config/config.go` (or equivalent config loader)
- Create: `internal/coordination/llm_summary.go`
- Test: `internal/coordination/llm_summary_test.go`

**Step 1: Write the failing test**

Create `internal/coordination/llm_summary_test.go`:

```go
func TestLLMSummaryCommand(t *testing.T) {
    tmp := t.TempDir()
    cmdPath := filepath.Join(tmp, "summary.sh")
    script := "#!/bin/sh\ncat >/tmp/input.json\necho '{\"summary\":{\"participants\":[\"alice\"],\"key_points\":[\"p1\"],\"action_items\":[]},\"examples\":[{\"id\":\"m1\",\"subject\":\"Hello\",\"body\":\"Body\"}]}'\n"
    if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
        t.Fatal(err)
    }

    cfg := LLMCommandConfig{Command: cmdPath, TimeoutSeconds: 5}
    input := LLMSummaryInput{ThreadID: "t1", Messages: []LLMMessage{{ID: "m1", Sender: "alice", Subject: "Hello", Body: "Body"}}}

    out, err := RunLLMSummaryCommand(context.Background(), cfg, input)
    if err != nil {
        t.Fatalf("run: %v", err)
    }
    if len(out.Summary.KeyPoints) != 1 || len(out.Examples) != 1 {
        t.Fatalf("expected summary and example")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/coordination -run TestLLMSummaryCommand -v`

Expected: FAIL with undefined types/functions.

**Step 3: Add config struct**

In `internal/config/config.go`, add:

```go
type LLMCommandConfig struct {
    Command string `json:"command" yaml:"command"`
    TimeoutSeconds int `json:"timeout_seconds" yaml:"timeout_seconds"`
}
```

And load it from config (`.tandemonium/config.yml`) under a `llm_summary` section.

**Step 4: Implement command runner**

Create `internal/coordination/llm_summary.go`:

```go
type LLMMessage struct { ID, Sender, Subject, Body string }

type LLMSummaryInput struct {
    ThreadID string `json:"thread_id"`
    Messages []LLMMessage `json:"messages"`
    IncludeExamples bool `json:"include_examples"`
}

type LLMSummaryOutput struct {
    Summary struct {
        Participants []string `json:"participants"`
        KeyPoints []string `json:"key_points"`
        ActionItems []string `json:"action_items"`
    } `json:"summary"`
    Examples []struct {
        ID string `json:"id"`
        Subject string `json:"subject"`
        Body string `json:"body"`
    } `json:"examples"`
}

func RunLLMSummaryCommand(ctx context.Context, cfg LLMCommandConfig, input LLMSummaryInput) (LLMSummaryOutput, error) {
    if strings.TrimSpace(cfg.Command) == "" {
        return LLMSummaryOutput{}, errors.New("llm summary command not configured")
    }
    timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
    if timeout <= 0 { timeout = 30 * time.Second }
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, cfg.Command)
    stdin, err := cmd.StdinPipe()
    if err != nil { return LLMSummaryOutput{}, err }
    go func() {
        _ = json.NewEncoder(stdin).Encode(input)
        _ = stdin.Close()
    }()
    var out bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = &out
    if err := cmd.Run(); err != nil {
        return LLMSummaryOutput{}, fmt.Errorf("llm command failed: %w: %s", err, out.String())
    }
    var payload LLMSummaryOutput
    if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
        return LLMSummaryOutput{}, err
    }
    return payload, nil
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/coordination -run TestLLMSummaryCommand -v`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/config/config.go internal/coordination/llm_summary.go internal/coordination/llm_summary_test.go
git commit -m "feat: add LLM summary command hook"
```

### Task 2: Wire LLM summaries into summarize_thread

**Files:**
- Modify: `internal/storage/coordination.go`
- Modify: `internal/coordination/compat.go`
- Modify: `internal/cli/commands/mail.go`
- Test: `internal/coordination/compat_test.go`

**Step 1: Write the failing test**

Add to `internal/coordination/compat_test.go`:

```go
func TestSummarizeThreadWithLLM(t *testing.T) {
    db, err := storage.OpenTemp()
    if err != nil { t.Fatal(err) }
    defer db.Close()
    if err := storage.Migrate(db); err != nil { t.Fatal(err) }

    if _, err := SendMessage(db, SendMessageRequest{MessageID: "m1", Sender: "alice", Subject: "Hello", Body: "Body", To: []string{"bob"}}); err != nil {
        t.Fatalf("send: %v", err)
    }

    cfg := LLMCommandConfig{Command: "./fixtures/llm-summary.sh", TimeoutSeconds: 5}
    resp, err := SummarizeThread(db, SummarizeThreadRequest{ThreadID: "m1", IncludeExamples: true, LLMMode: true, LLMConfig: cfg})
    if err != nil { t.Fatalf("summarize: %v", err) }
    if len(resp.Examples) == 0 { t.Fatalf("expected examples") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/coordination -run TestSummarizeThreadWithLLM -v`

Expected: FAIL with missing fields and types.

**Step 3: Implement LLM summary path**

In `internal/coordination/compat.go`:
- Extend `SummarizeThreadRequest` with `IncludeExamples bool`, `LLMMode bool`, and `LLMConfig`.
- Extend `SummarizeThreadResponse` to include `Examples` and `KeyPoints`/`ActionItems` if LLM mode used.
- When LLM mode enabled, load thread messages (new helper in storage), call `RunLLMSummaryCommand`, and map output fields.

In `internal/storage/coordination.go`:
- Add `ListThreadMessages(db, threadID string, limit int) ([]Message, error)` for LLM input.

**Step 4: Wire CLI flags**

In `internal/cli/commands/mail.go`:
- Add `--llm` and `--examples` flags to `mail summarize`.
- Load config and pass into `SummarizeThread` with LLM settings.

**Step 5: Run tests**

Run:
- `go test ./internal/coordination -run TestSummarizeThreadWithLLM -v`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/storage/coordination.go internal/coordination/compat.go internal/coordination/compat_test.go internal/cli/commands/mail.go
git commit -m "feat: add LLM-backed thread summaries"
```

### Task 3: Documentation (default Claude command)

**Files:**
- Modify: `docs/plans/2026-01-16-mcp-compatibility.md`
- Modify: `README.md` or `docs/` (config example)

**Step 1: Add config example**

Example config:

```yaml
llm_summary:
  command: "claude"
  timeout_seconds: 30
```

**Step 2: Update parity note**

Remove the remaining gap from MCP compatibility notes.

**Step 3: Commit**

```bash
git add docs/plans/2026-01-16-mcp-compatibility.md README.md
git commit -m "docs: document llm summary command hook"
```
