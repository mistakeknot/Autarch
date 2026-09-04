package door

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Conversation is evidence read from a transcript, independent of whether
// its process is alive. Text is quoted, never treated as verified delivery.
type Conversation struct {
	Provider       Runtime   `json:"provider"`
	Source         string    `json:"source"`
	Updated        time.Time `json:"updated"`
	Request        string    `json:"request,omitempty"`
	Report         string    `json:"report,omitempty"`
	Question       string    `json:"question,omitempty"`
	Context        string    `json:"context,omitempty"`
	Reply          string    `json:"later_reply,omitempty"` // a reply is not proof this question was answered
	WorkDir        string    `json:"work_dir,omitempty"`
	QuestionAt     time.Time `json:"question_at,omitempty"`
	QuestionLine   int       `json:"question_line,omitempty"` // zero when reading a tail
	QuestionOffset int64     `json:"question_offset,omitempty"`
	questionCall   string
	async          bool
}

const conversationTailBytes int64 = 4 << 20

type conversationBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type conversationMessage struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Channel   string          `json:"channel"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	CallID    string          `json:"call_id"`
	Arguments string          `json:"arguments"`
	Output    json.RawMessage `json:"output"`
	CWD       string          `json:"cwd"`
}

// ReadConversation bounds I/O even for months-long standing threads. Byte
// offsets are absolute; line numbers are supplied only for a full-file read.
func ReadConversation(path string, provider Runtime) (Conversation, error) {
	c := Conversation{Source: path, Provider: provider}
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return c, err
	}
	offset := int64(0)
	if st.Size() > conversationTailBytes {
		offset = st.Size() - conversationTailBytes
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return c, err
	}
	r := bufio.NewReaderSize(io.LimitReader(f, st.Size()-offset), 64<<10)
	if offset > 0 {
		b, _ := r.ReadBytes('\n')
		offset += int64(len(b))
	}
	lineNumber := 0
	full := offset == 0
	for {
		line, readErr := r.ReadBytes('\n')
		lineNumber++
		if len(line) > 0 {
			var row struct {
				Type      string              `json:"type"`
				Timestamp string              `json:"timestamp"`
				IsMeta    bool                `json:"isMeta"`
				CWD       string              `json:"cwd"`
				Message   json.RawMessage     `json:"message"`
				Payload   conversationMessage `json:"payload"`
			}
			if json.Unmarshal(line, &row) == nil && !row.IsMeta {
				if row.CWD != "" {
					c.WorkDir = row.CWD
				}
				if provider == RuntimeCodex && (row.Type == "session_meta" || row.Type == "turn_context") && row.Payload.CWD != "" {
					c.WorkDir = row.Payload.CWD
				}
				ts, _ := time.Parse(time.RFC3339, row.Timestamp)
				ln := 0
				if full {
					ln = lineNumber
				}
				if row.Type == "response_item" && provider == RuntimeCodex {
					c.consume(row.Payload, ts, ln, offset)
				} else if (row.Type == "user" || row.Type == "assistant") && provider == RuntimeClaude {
					var msg conversationMessage
					if json.Unmarshal(row.Message, &msg) != nil {
						msg.Content = row.Message
					}
					msg.Type = "message"
					msg.Role = row.Type
					c.consume(msg, ts, ln, offset)
				}
			}
			offset += int64(len(line))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return c, readErr
		}
	}
	return c, nil
}

func (c *Conversation) clearQuestion() {
	c.Question = ""
	c.Context = ""
	c.Reply = ""
	c.QuestionAt = time.Time{}
	c.QuestionLine = 0
	c.QuestionOffset = 0
	c.questionCall = ""
	c.async = false
}

func (c *Conversation) ask(question, call string, async bool, at time.Time, line int, offset int64) {
	if question == "" {
		return
	}
	c.Question = cleanEvidence(question)
	c.Context = c.Report
	c.Reply = ""
	c.questionCall = call
	c.async = async
	c.QuestionAt = at
	c.QuestionLine = line
	c.QuestionOffset = offset
	if !at.IsZero() {
		c.Updated = at
	}
}

func (c *Conversation) consume(msg conversationMessage, at time.Time, line int, offset int64) {
	if msg.Type == "function_call" && isQuestionTool(msg.Name) {
		c.ask(questionText([]byte(msg.Arguments)), msg.CallID, strings.HasSuffix(msg.Name, "_async"), at, line, offset)
		return
	}
	if msg.Type == "function_call_output" && msg.CallID == c.questionCall && !c.async {
		// A synchronous question tool's empty/cancelled result is not an answer.
		var out string
		_ = json.Unmarshal(msg.Output, &out)
		var answer struct {
			Answers map[string]json.RawMessage `json:"answers"`
		}
		if json.Unmarshal([]byte(out), &answer) == nil && len(answer.Answers) > 0 {
			complete := true
			for _, raw := range answer.Answers {
				var a struct {
					Answers []string `json:"answers"`
				}
				if json.Unmarshal(raw, &a) != nil || len(a.Answers) == 0 {
					complete = false
					break
				}
				for _, text := range a.Answers {
					if strings.TrimSpace(text) == "" {
						complete = false
					}
				}
			}
			if complete {
				c.clearQuestion()
			}
		}
		return
	}
	if msg.Type != "message" || (msg.Role != "assistant" && msg.Role != "user") {
		return
	}
	var blocks []conversationBlock
	var text string
	if json.Unmarshal(msg.Content, &text) != nil {
		_ = json.Unmarshal(msg.Content, &blocks)
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" || b.Type == "input_text" || b.Type == "output_text" {
				parts = append(parts, b.Text)
			}
		}
		text = strings.Join(parts, "\n\n")
	}
	text = cleanEvidence(text)
	if msg.Role == "user" {
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID == c.questionCall && !b.IsError {
				var answer string
				_ = json.Unmarshal(b.Content, &answer)
				if strings.Contains(answer, "User has answered") {
					c.clearQuestion()
				}
			}
		}
		if isHumanText(text) {
			if c.Question != "" {
				c.Reply = text
			} else {
				c.Request = text
			}
			if !at.IsZero() {
				c.Updated = at
			}
		}
		return
	}
	if !at.IsZero() {
		c.Updated = at
	}
	// Reasoning is not a report intended for the user.
	if msg.Channel == "analysis" {
		return
	}
	if text != "" {
		if q := directQuestion(text); q != "" {
			c.ask(q, "", false, at, line, offset)
			if context := strings.TrimSpace(strings.TrimSuffix(text, q)); context != "" {
				c.Context = context
			}
		}
		c.Report = text
	}
	for _, b := range blocks {
		if b.Type == "tool_use" && isQuestionTool(b.Name) {
			c.ask(questionText(b.Input), b.ID, false, at, line, offset)
		}
	}
}

func isQuestionTool(name string) bool {
	return name == "AskUserQuestion" || strings.HasSuffix(name, "request_user_input") || strings.HasSuffix(name, "request_user_input_async")
}

func questionText(raw []byte) string {
	var d struct {
		Questions []struct {
			Question string            `json:"question"`
			Title    string            `json:"title"`
			Options  []json.RawMessage `json:"options"`
		} `json:"questions"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return ""
	}
	var qs []string
	for _, q := range d.Questions {
		title := q.Question
		if title == "" {
			title = q.Title
		}
		if title == "" {
			continue
		}
		lines := []string{title}
		for _, o := range q.Options {
			var label string
			if json.Unmarshal(o, &label) != nil {
				var opt struct{ Label, Description string }
				_ = json.Unmarshal(o, &opt)
				label = opt.Label
				if opt.Description != "" {
					label += " — " + opt.Description
				}
			}
			if label != "" {
				lines = append(lines, "• "+label)
			}
		}
		qs = append(qs, strings.Join(lines, "\n"))
	}
	return strings.Join(qs, "\n\n")
}

func cleanEvidence(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(s)))
}

func isHumanText(s string) bool {
	if s == "" {
		return false
	}
	for _, prefix := range []string{"<", "# AGENTS.md", "Stop hook feedback:", "A session-scoped Stop hook", "This session is being continued", "[Request interrupted", "Base directory for this skill:"} {
		if strings.HasPrefix(s, prefix) {
			return false
		}
	}
	return true
}

// A question mark inside a report is insufficient. Only a direct final
// paragraph ending in a question is surfaced, always with its original text.
func directQuestion(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "\n\n")
	last := strings.TrimSpace(parts[len(parts)-1])
	if strings.HasSuffix(last, "?") && len(last) < 2000 {
		return last
	}
	return ""
}

var conversationID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func DefaultCodexRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

func FindConversation(claudeRoot, codexRoot, id string) (string, Runtime, error) {
	if !conversationID.MatchString(id) {
		return "", "", fmt.Errorf("invalid conversation id %q", id)
	}
	if claudeRoot != "" {
		if p, err := FindTranscript(claudeRoot, id); err == nil {
			return p, RuntimeClaude, nil
		}
	}
	if codexRoot != "" {
		matches, err := filepath.Glob(filepath.Join(codexRoot, "*", "*", "*", "*"+id+".jsonl"))
		if err != nil {
			return "", "", err
		}
		if len(matches) == 1 {
			return matches[0], RuntimeCodex, nil
		}
		if len(matches) > 1 {
			return "", "", fmt.Errorf("ambiguous Codex conversation %s", id)
		}
	}
	return "", "", fmt.Errorf("no transcript for %s", id)
}
