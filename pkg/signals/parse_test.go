package signals

import (
	"testing"
	"time"
)

func TestIsValidAgentState(t *testing.T) {
	tests := []struct {
		state AgentState
		want  bool
	}{
		{AgentWorking, true},
		{AgentNeedsInput, true},
		{AgentNeedsTesting, true},
		{AgentCompleted, true},
		{AgentError, true},
		{"unknown", false},
		{"COMPLETED", false},       // case-sensitive
		{"Working", false},         // case-sensitive
		{"needs_approval", false},  // not a valid state
	}
	for _, tt := range tests {
		if got := IsValidAgentState(tt.state); got != tt.want {
			t.Errorf("IsValidAgentState(%q) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestParseAgentSignals(t *testing.T) {
	fixed := time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		data string
		want []AgentSignal
	}{
		// Basic parsing
		{
			name: "single working signal",
			data: "--<[autarch:working:processing files]>--\n",
			want: []AgentSignal{{State: AgentWorking, Message: "processing files", Timestamp: fixed}},
		},
		{
			name: "single completed signal",
			data: "--<[autarch:completed:all tests pass]>--\n",
			want: []AgentSignal{{State: AgentCompleted, Message: "all tests pass", Timestamp: fixed}},
		},
		{
			name: "single error signal",
			data: "--<[autarch:error:build failed on line 42]>--\n",
			want: []AgentSignal{{State: AgentError, Message: "build failed on line 42", Timestamp: fixed}},
		},
		{
			name: "needs_input signal",
			data: "--<[autarch:needs_input:which database driver?]>--\n",
			want: []AgentSignal{{State: AgentNeedsInput, Message: "which database driver?", Timestamp: fixed}},
		},
		{
			name: "needs_testing signal",
			data: "--<[autarch:needs_testing:login flow ready for review]>--\n",
			want: []AgentSignal{{State: AgentNeedsTesting, Message: "login flow ready for review", Timestamp: fixed}},
		},
		{
			name: "empty message",
			data: "--<[autarch:working:]>--\n",
			want: []AgentSignal{{State: AgentWorking, Message: "", Timestamp: fixed}},
		},
		{
			name: "message with colons",
			data: "--<[autarch:error:failed at step 3: timeout after 30s]>--\n",
			want: []AgentSignal{{State: AgentError, Message: "failed at step 3: timeout after 30s", Timestamp: fixed}},
		},

		// Multi-signal
		{
			name: "multiple signals in buffer",
			data: "--<[autarch:working:step 1]>--\nsome output\n--<[autarch:completed:done]>--\n",
			want: []AgentSignal{
				{State: AgentWorking, Message: "step 1", Timestamp: fixed},
				{State: AgentCompleted, Message: "done", Timestamp: fixed},
			},
		},
		{
			name: "mix of valid and invalid states",
			data: "--<[autarch:working:ok]>--\n--<[autarch:INVALID:nope]>--\n--<[autarch:error:fail]>--\n",
			want: []AgentSignal{
				{State: AgentWorking, Message: "ok", Timestamp: fixed},
				{State: AgentError, Message: "fail", Timestamp: fixed},
			},
		},

		// Whitespace and line endings
		{
			name: "leading whitespace",
			data: "   --<[autarch:working:ok]>--\n",
			want: []AgentSignal{{State: AgentWorking, Message: "ok", Timestamp: fixed}},
		},
		{
			name: "trailing whitespace",
			data: "--<[autarch:working:ok]>--   \n",
			want: []AgentSignal{{State: AgentWorking, Message: "ok", Timestamp: fixed}},
		},
		{
			name: "windows line endings",
			data: "--<[autarch:working:ok]>--\r\n",
			want: []AgentSignal{{State: AgentWorking, Message: "ok", Timestamp: fixed}},
		},
		{
			name: "leading tabs",
			data: "\t\t--<[autarch:completed:done]>--\n",
			want: []AgentSignal{{State: AgentCompleted, Message: "done", Timestamp: fixed}},
		},

		// Bullet prefixes (Claude Code, markdown)
		{
			name: "claude code bullet prefix",
			data: "⏺ --<[autarch:working:scanning]>--\n",
			want: []AgentSignal{{State: AgentWorking, Message: "scanning", Timestamp: fixed}},
		},
		{
			name: "markdown bullet prefix",
			data: "• --<[autarch:completed:done]>--\n",
			want: []AgentSignal{{State: AgentCompleted, Message: "done", Timestamp: fixed}},
		},
		{
			name: "dash bullet prefix",
			data: "- --<[autarch:error:fail]>--\n",
			want: []AgentSignal{{State: AgentError, Message: "fail", Timestamp: fixed}},
		},
		{
			name: "asterisk bullet prefix",
			data: "* --<[autarch:working:ok]>--\n",
			want: []AgentSignal{{State: AgentWorking, Message: "ok", Timestamp: fixed}},
		},

		// Rejection cases
		{
			name: "no signals in plain text",
			data: "just some normal output\nwith multiple lines\n",
			want: nil,
		},
		{
			name: "empty input",
			data: "",
			want: nil,
		},
		{
			name: "inline with text before - not matched",
			data: "output--<[autarch:working:ok]>--\n",
			want: nil,
		},
		{
			name: "inline with text after - not matched",
			data: "--<[autarch:working:ok]>--more\n",
			want: nil,
		},
		{
			name: "missing closing delimiter",
			data: "--<[autarch:working:ok\n",
			want: nil,
		},
		{
			name: "missing opening delimiter",
			data: "autarch:working:ok]>--\n",
			want: nil,
		},
		{
			name: "wrong namespace",
			data: "--<[schmux:working:ok]>--\n",
			want: nil,
		},
		{
			name: "invalid state rejected",
			data: "--<[autarch:COMPLETED:ok]>--\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAgentSignalsAt([]byte(tt.data), fixed)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d signals, want %d\ngot:  %+v\nwant: %+v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i].State != tt.want[i].State {
					t.Errorf("signal[%d].State = %q, want %q", i, got[i].State, tt.want[i].State)
				}
				if got[i].Message != tt.want[i].Message {
					t.Errorf("signal[%d].Message = %q, want %q", i, got[i].Message, tt.want[i].Message)
				}
				if !got[i].Timestamp.Equal(tt.want[i].Timestamp) {
					t.Errorf("signal[%d].Timestamp = %v, want %v", i, got[i].Timestamp, tt.want[i].Timestamp)
				}
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no escape sequences",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "color codes removed",
			input: "\x1b[32mgreen\x1b[0m",
			want:  "green",
		},
		{
			name:  "cursor forward replaced with space",
			input: "word\x1b[1Cword",
			want:  "word word",
		},
		{
			name:  "cursor down replaced with newline",
			input: "line1\x1b[1Bline2",
			want:  "line1\nline2",
		},
		{
			name:  "OSC sequence removed (BEL terminator)",
			input: "text\x1b]0;window title\x07more",
			want:  "textmore",
		},
		{
			name:  "OSC sequence removed (ST terminator)",
			input: "text\x1b]0;title\x1b\\more",
			want:  "textmore",
		},
		{
			name:  "DEC private mode removed",
			input: "\x1b[?2026l\x1b[?2026htext",
			want:  "text",
		},
		{
			name:  "complex mix",
			input: "\x1b[?2026l\x1b[38;2;255;255;255mhello\x1b[1C\x1b[39mworld\x1b[1B",
			want:  "hello world\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.input)
			if got != tt.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseAgentSignalsWithANSI(t *testing.T) {
	fixed := time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		data string
		want []AgentSignal
	}{
		{
			name: "signal with color codes",
			data: "\x1b[32m--<[autarch:working:ok]>--\x1b[0m\n",
			want: []AgentSignal{{State: AgentWorking, Message: "ok", Timestamp: fixed}},
		},
		{
			name: "signal with DEC private mode prefix",
			data: "\x1b[?2026l\x1b[?2026h--<[autarch:completed:done]>--\n",
			want: []AgentSignal{{State: AgentCompleted, Message: "done", Timestamp: fixed}},
		},
		{
			name: "message with cursor forward spaces",
			data: "--<[autarch:needs_input:How\x1b[1Ccan\x1b[1CI\x1b[1Chelp]>--\n",
			want: []AgentSignal{{State: AgentNeedsInput, Message: "How can I help", Timestamp: fixed}},
		},
		{
			name: "claude code bullet with full ANSI garble",
			data: "\r\n\x1b[?2026l\x1b[?2026h\r\x1b[38;2;255;255;255m\xe2\x8f\xba\x1b[1C\x1b[39m--<[autarch:needs_input:How\x1b[1Ccan\x1b[1CI\x1b[1Chelp]>--\r\n",
			want: []AgentSignal{{State: AgentNeedsInput, Message: "How can I help", Timestamp: fixed}},
		},
		{
			name: "signal buried in output with ANSI",
			data: "\x1b[32mBuilding...\x1b[0m\nOK\n--<[autarch:completed:build succeeded]>--\n\x1b[33mDone.\x1b[0m\n",
			want: []AgentSignal{{State: AgentCompleted, Message: "build succeeded", Timestamp: fixed}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAgentSignalsAt([]byte(tt.data), fixed)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d signals, want %d\ngot:  %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i].State != tt.want[i].State {
					t.Errorf("signal[%d].State = %q, want %q", i, got[i].State, tt.want[i].State)
				}
				if got[i].Message != tt.want[i].Message {
					t.Errorf("signal[%d].Message = %q, want %q", i, got[i].Message, tt.want[i].Message)
				}
			}
		})
	}
}
