package agenttargets

// StreamEventType represents the kind of streaming event from an agent backend.
type StreamEventType int

const (
	StreamText          StreamEventType = iota // Assistant text content delta
	StreamThinkingStart                        // Extended thinking started
	StreamThinking                             // Extended thinking content delta
	StreamThinkingEnd                          // Extended thinking ended
	StreamToolUse                              // Tool invocation (name + input)
	StreamResult                               // Final result text
	StreamError                                // Error occurred
	StreamSessionID                            // Session ID extracted from init message
)

// StreamEvent is a single backend-agnostic event from a streaming agent session.
type StreamEvent struct {
	Type      StreamEventType
	Text      string         // Content for text/thinking/result/error events
	ToolName  string         // For StreamToolUse
	ToolInput map[string]any // For StreamToolUse
	SessionID string         // For StreamSessionID
	IsError   bool           // For StreamResult — whether the agent flagged it as error
	Backend   string         // "claude", "codex"
}

// StreamHandle provides control over a streaming agent process.
type StreamHandle struct {
	*RunHandle
	Events <-chan StreamEvent
}
