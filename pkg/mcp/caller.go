package mcp

import (
	"context"
	"os"
	"strings"
)

type callerKey struct{}

// CallerInfo represents MCP caller identity and granted scopes.
type CallerInfo struct {
	AgentID string
	Scopes  []string // e.g. ["read"], ["read","write"], ["admin"]
}

// WithCaller stores caller identity in context.
func WithCaller(ctx context.Context, caller CallerInfo) context.Context {
	return context.WithValue(ctx, callerKey{}, caller)
}

// CallerFromContext loads caller identity from context.
func CallerFromContext(ctx context.Context) (CallerInfo, bool) {
	v, ok := ctx.Value(callerKey{}).(CallerInfo)
	return v, ok
}

// HasScope returns true when caller has the required scope.
// "admin" implies all scopes.
func (c CallerInfo) HasScope(required string) bool {
	required = strings.TrimSpace(strings.ToLower(required))
	if required == "" {
		return true
	}
	for _, scope := range c.Scopes {
		s := strings.TrimSpace(strings.ToLower(scope))
		if s == "admin" || s == required {
			return true
		}
	}
	return false
}

// StdioCaller is the explicit caller for the stdio entry points. The process is started
// by the local agent that owns the pipe, so it is trusted with read and write, and never
// admin. Without a caller in the context a scoped tool is denied, so an entry point that
// forgets to attach one fails closed instead of skipping enforcement.
func StdioCaller() CallerInfo {
	id := strings.TrimSpace(os.Getenv("BB_THREAD_ID"))
	if id == "" {
		id = "stdio"
	}
	return CallerInfo{AgentID: id, Scopes: []string{"read", "write"}}
}
