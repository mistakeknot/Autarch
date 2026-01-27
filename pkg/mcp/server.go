// Package mcp provides an MCP (Model Context Protocol) server for Autarch.
// This allows AI agents to interact with Autarch tools programmatically.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// Server implements the MCP protocol for Autarch tools.
type Server struct {
	projectPath string
	tools       map[string]Tool
	mu          sync.RWMutex

	// I/O for JSON-RPC communication
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// NewServer creates a new MCP server.
func NewServer(projectPath string) *Server {
	s := &Server{
		projectPath: projectPath,
		tools:       make(map[string]Tool),
		stdin:       os.Stdin,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
	}
	s.registerDefaultTools()
	return s
}

// WithIO sets custom I/O streams (for testing).
func (s *Server) WithIO(stdin io.Reader, stdout, stderr io.Writer) *Server {
	s.stdin = stdin
	s.stdout = stdout
	s.stderr = stderr
	return s
}

// Tool represents an MCP tool that can be invoked by agents.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Handler     ToolHandler            `json:"-"`
}

// ToolHandler is the function signature for tool implementations.
type ToolHandler func(ctx context.Context, params map[string]interface{}) (interface{}, error)

// RegisterTool adds a tool to the server.
func (s *Server) RegisterTool(tool Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
}

// registerDefaultTools adds the standard Autarch tools.
func (s *Server) registerDefaultTools() {
	s.RegisterTool(Tool{
		Name:        "autarch_list_prds",
		Description: "List all PRD specifications in the project",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Filter by status (draft, review, approved, implemented)",
				},
			},
		},
		Handler: s.handleListPRDs,
	})

	s.RegisterTool(Tool{
		Name:        "autarch_get_prd",
		Description: "Get a specific PRD by ID",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "PRD ID (e.g., PRD-001)",
				},
			},
		},
		Handler: s.handleGetPRD,
	})

	s.RegisterTool(Tool{
		Name:        "autarch_list_tasks",
		Description: "List Coldwine tasks for a PRD or epic",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prd_id": map[string]interface{}{
					"type":        "string",
					"description": "Filter by PRD ID",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Filter by status (pending, in_progress, blocked, completed)",
				},
			},
		},
		Handler: s.handleListTasks,
	})

	s.RegisterTool(Tool{
		Name:        "autarch_update_task",
		Description: "Update a Coldwine task status",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"id", "status"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "New status (pending, in_progress, blocked, completed)",
				},
				"note": map[string]interface{}{
					"type":        "string",
					"description": "Optional status update note",
				},
			},
		},
		Handler: s.handleUpdateTask,
	})

	s.RegisterTool(Tool{
		Name:        "autarch_research",
		Description: "Run Pollard research on a topic",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Research query or topic",
				},
				"hunters": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Specific hunters to use (optional)",
				},
			},
		},
		Handler: s.handleResearch,
	})

	s.RegisterTool(Tool{
		Name:        "autarch_suggest_hunters",
		Description: "Get recommended Pollard hunters for a topic",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Topic to research",
				},
			},
		},
		Handler: s.handleSuggestHunters,
	})

	s.RegisterTool(Tool{
		Name:        "autarch_project_status",
		Description: "Get Bigend project status aggregation",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"include_details": map[string]interface{}{
					"type":        "boolean",
					"description": "Include detailed breakdown",
				},
			},
		},
		Handler: s.handleProjectStatus,
	})

	s.RegisterTool(Tool{
		Name:        "autarch_send_message",
		Description: "Send a message via Intermute",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"to", "subject", "body"},
			"properties": map[string]interface{}{
				"to": map[string]interface{}{
					"type":        "string",
					"description": "Recipient tool (gurgeh, coldwine, pollard, bigend)",
				},
				"subject": map[string]interface{}{
					"type":        "string",
					"description": "Message subject",
				},
				"body": map[string]interface{}{
					"type":        "string",
					"description": "Message body",
				},
			},
		},
		Handler: s.handleSendMessage,
	})
}

// Run starts the MCP server's main loop.
func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			s.handleRequest(ctx, &req)
		}
	}

	return scanner.Err()
}

// handleRequest processes a single JSON-RPC request.
func (s *Server) handleRequest(ctx context.Context, req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	case "shutdown":
		s.handleShutdown(req)
	default:
		s.sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

func (s *Server) handleInitialize(req *JSONRPCRequest) {
	resp := InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: ServerInfo{
			Name:    "autarch",
			Version: "0.1.0",
		},
		Capabilities: Capabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
	}
	s.sendResult(req.ID, resp)
}

func (s *Server) handleToolsList(req *JSONRPCRequest) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	s.sendResult(req.ID, ToolsListResult{Tools: tools})
}

func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) {
	var params ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	s.mu.RLock()
	tool, ok := s.tools[params.Name]
	s.mu.RUnlock()

	if !ok {
		s.sendError(req.ID, -32602, "Unknown tool", params.Name)
		return
	}

	result, err := tool.Handler(ctx, params.Arguments)
	if err != nil {
		s.sendResult(req.ID, ToolsCallResult{
			Content: []ContentBlock{
				{Type: "text", Text: "Error: " + err.Error()},
			},
			IsError: true,
		})
		return
	}

	// Convert result to text
	var text string
	switch v := result.(type) {
	case string:
		text = v
	default:
		data, _ := json.MarshalIndent(result, "", "  ")
		text = string(data)
	}

	s.sendResult(req.ID, ToolsCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	})
}

func (s *Server) handleShutdown(req *JSONRPCRequest) {
	s.sendResult(req.ID, nil)
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintln(s.stdout, string(data))
}

func (s *Server) sendError(id interface{}, code int, message, data string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	respData, _ := json.Marshal(resp)
	fmt.Fprintln(s.stdout, string(respData))
}
