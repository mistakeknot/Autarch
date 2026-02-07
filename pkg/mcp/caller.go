package mcp

import (
	"context"
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
