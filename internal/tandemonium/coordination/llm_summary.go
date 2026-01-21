package coordination

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
)

type LLMMessage struct {
	ID      string `json:"id"`
	Sender  string `json:"sender"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type LLMSummaryInput struct {
	ThreadID        string       `json:"thread_id"`
	Messages        []LLMMessage `json:"messages"`
	IncludeExamples bool         `json:"include_examples"`
}

type LLMSummaryExample struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type LLMSummaryOutput struct {
	Summary struct {
		Participants []string `json:"participants"`
		KeyPoints    []string `json:"key_points"`
		ActionItems  []string `json:"action_items"`
	} `json:"summary"`
	Examples []LLMSummaryExample `json:"examples"`
}

func RunLLMSummaryCommand(ctx context.Context, cfg config.LLMSummaryConfig, input LLMSummaryInput) (LLMSummaryOutput, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return LLMSummaryOutput{}, errors.New("llm summary command not configured")
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return LLMSummaryOutput{}, err
	}
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
		return LLMSummaryOutput{}, fmt.Errorf("llm command output invalid: %w", err)
	}
	return payload, nil
}
