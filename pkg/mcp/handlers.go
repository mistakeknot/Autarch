package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Tool handlers for Autarch MCP server

func (s *Server) handleListPRDs(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	specsDir := filepath.Join(s.projectPath, ".gurgeh", "specs")

	entries, err := os.ReadDir(specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{
				"prds":  []interface{}{},
				"count": 0,
			}, nil
		}
		return nil, fmt.Errorf("failed to read specs directory: %w", err)
	}

	statusFilter := ""
	if status, ok := params["status"].(string); ok {
		statusFilter = status
	}

	var prds []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		prdPath := filepath.Join(specsDir, entry.Name())
		prd, err := readPRDSummary(prdPath)
		if err != nil {
			continue
		}

		// Apply status filter
		if statusFilter != "" {
			if prdStatus, ok := prd["status"].(string); ok && prdStatus != statusFilter {
				continue
			}
		}

		prds = append(prds, prd)
	}

	return map[string]interface{}{
		"prds":  prds,
		"count": len(prds),
	}, nil
}

func (s *Server) handleGetPRD(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("id parameter is required")
	}

	// Normalize ID to filename
	filename := id
	if !strings.HasSuffix(filename, ".yaml") {
		filename = filename + ".yaml"
	}

	prdPath := filepath.Join(s.projectPath, ".gurgeh", "specs", filename)
	data, err := os.ReadFile(prdPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("PRD not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read PRD: %w", err)
	}

	var prd map[string]interface{}
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return nil, fmt.Errorf("failed to parse PRD: %w", err)
	}

	return prd, nil
}

func (s *Server) handleListTasks(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	tasksDir := filepath.Join(s.projectPath, ".coldwine", "tasks")

	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{
				"tasks": []interface{}{},
				"count": 0,
			}, nil
		}
		return nil, fmt.Errorf("failed to read tasks directory: %w", err)
	}

	prdFilter := ""
	statusFilter := ""
	if prd, ok := params["prd_id"].(string); ok {
		prdFilter = prd
	}
	if status, ok := params["status"].(string); ok {
		statusFilter = status
	}

	var tasks []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		taskPath := filepath.Join(tasksDir, entry.Name())
		task, err := readTaskSummary(taskPath)
		if err != nil {
			continue
		}

		// Apply filters
		if prdFilter != "" {
			if taskPRD, ok := task["prd_id"].(string); ok && taskPRD != prdFilter {
				continue
			}
		}
		if statusFilter != "" {
			if taskStatus, ok := task["status"].(string); ok && taskStatus != statusFilter {
				continue
			}
		}

		tasks = append(tasks, task)
	}

	return map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	}, nil
}

func (s *Server) handleUpdateTask(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("id parameter is required")
	}

	newStatus, ok := params["status"].(string)
	if !ok || newStatus == "" {
		return nil, fmt.Errorf("status parameter is required")
	}

	// Validate status
	validStatuses := map[string]bool{
		"pending": true, "in_progress": true, "blocked": true, "completed": true,
	}
	if !validStatuses[newStatus] {
		return nil, fmt.Errorf("invalid status: %s", newStatus)
	}

	// Read existing task
	filename := id
	if !strings.HasSuffix(filename, ".yaml") {
		filename = filename + ".yaml"
	}
	taskPath := filepath.Join(s.projectPath, ".coldwine", "tasks", filename)

	data, err := os.ReadFile(taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("task not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read task: %w", err)
	}

	var task map[string]interface{}
	if err := yaml.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to parse task: %w", err)
	}

	// Update status
	oldStatus := task["status"]
	task["status"] = newStatus

	// Add note if provided
	if note, ok := params["note"].(string); ok && note != "" {
		updates, _ := task["status_updates"].([]interface{})
		updates = append(updates, map[string]interface{}{
			"from": oldStatus,
			"to":   newStatus,
			"note": note,
		})
		task["status_updates"] = updates
	}

	// Write back
	newData, err := yaml.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize task: %w", err)
	}

	if err := os.WriteFile(taskPath, newData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write task: %w", err)
	}

	return map[string]interface{}{
		"success":    true,
		"id":         id,
		"old_status": oldStatus,
		"new_status": newStatus,
	}, nil
}

func (s *Server) handleResearch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required")
	}

	// Get specified hunters or use defaults
	var hunters []string
	if h, ok := params["hunters"].([]interface{}); ok {
		for _, item := range h {
			if s, ok := item.(string); ok {
				hunters = append(hunters, s)
			}
		}
	}

	// For now, return a placeholder - actual implementation would call Pollard
	return map[string]interface{}{
		"status":  "queued",
		"query":   query,
		"hunters": hunters,
		"message": "Research request queued. Use autarch_project_status to check progress.",
	}, nil
}

func (s *Server) handleSuggestHunters(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required")
	}

	lower := strings.ToLower(query)

	// Simple keyword-based hunter suggestion
	suggestions := []map[string]interface{}{}

	if containsAny(lower, []string{"github", "repo", "code", "library", "package"}) {
		suggestions = append(suggestions, map[string]interface{}{
			"hunter":      "github-scout",
			"description": "Searches GitHub for repositories and code",
			"relevance":   "high",
		})
	}

	if containsAny(lower, []string{"paper", "research", "academic", "study", "journal"}) {
		suggestions = append(suggestions, map[string]interface{}{
			"hunter":      "openalex",
			"description": "Searches academic papers and citations",
			"relevance":   "high",
		})
	}

	if containsAny(lower, []string{"medical", "health", "drug", "clinical", "disease"}) {
		suggestions = append(suggestions, map[string]interface{}{
			"hunter":      "pubmed",
			"description": "Searches medical and biomedical literature",
			"relevance":   "high",
		})
	}

	if containsAny(lower, []string{"framework", "docs", "documentation", "api"}) {
		suggestions = append(suggestions, map[string]interface{}{
			"hunter":      "context7",
			"description": "Fetches framework documentation from Context7",
			"relevance":   "high",
		})
	}

	if containsAny(lower, []string{"law", "legal", "court", "case"}) {
		suggestions = append(suggestions, map[string]interface{}{
			"hunter":      "courtlistener",
			"description": "Searches court cases and legal opinions",
			"relevance":   "high",
		})
	}

	if containsAny(lower, []string{"patent", "invention", "ip"}) {
		suggestions = append(suggestions, map[string]interface{}{
			"hunter":      "patents-view",
			"description": "Searches USPTO patent database",
			"relevance":   "high",
		})
	}

	// Default suggestion
	if len(suggestions) == 0 {
		suggestions = append(suggestions, map[string]interface{}{
			"hunter":      "web-searcher",
			"description": "General web search",
			"relevance":   "medium",
		})
	}

	return map[string]interface{}{
		"query":       query,
		"suggestions": suggestions,
	}, nil
}

func (s *Server) handleProjectStatus(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	includeDetails := false
	if d, ok := params["include_details"].(bool); ok {
		includeDetails = d
	}

	// Count PRDs by status
	prdStats := countFilesByStatus(filepath.Join(s.projectPath, ".gurgeh", "specs"), "status")

	// Count tasks by status
	taskStats := countFilesByStatus(filepath.Join(s.projectPath, ".coldwine", "tasks"), "status")

	// Count research insights
	insightCount := countFiles(filepath.Join(s.projectPath, ".pollard", "insights"))

	result := map[string]interface{}{
		"project": s.projectPath,
		"prds": map[string]interface{}{
			"total":    sumValues(prdStats),
			"by_status": prdStats,
		},
		"tasks": map[string]interface{}{
			"total":    sumValues(taskStats),
			"by_status": taskStats,
		},
		"research": map[string]interface{}{
			"insights": insightCount,
		},
	}

	if includeDetails {
		// Add recent activity, etc.
		result["details"] = map[string]interface{}{
			"message": "Detailed stats not yet implemented",
		}
	}

	return result, nil
}

func (s *Server) handleSendMessage(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	to, ok := params["to"].(string)
	if !ok || to == "" {
		return nil, fmt.Errorf("to parameter is required")
	}

	subject, ok := params["subject"].(string)
	if !ok || subject == "" {
		return nil, fmt.Errorf("subject parameter is required")
	}

	body, ok := params["body"].(string)
	if !ok || body == "" {
		return nil, fmt.Errorf("body parameter is required")
	}

	// Validate recipient
	validRecipients := map[string]bool{
		"gurgeh": true, "coldwine": true, "pollard": true, "bigend": true,
	}
	if !validRecipients[to] {
		return nil, fmt.Errorf("invalid recipient: %s (must be gurgeh, coldwine, pollard, or bigend)", to)
	}

	// Write message to Intermute queue
	intermuteDir := filepath.Join(s.projectPath, ".intermute", "queues", to)
	if err := os.MkdirAll(intermuteDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create queue directory: %w", err)
	}

	// Generate message ID
	msgID := fmt.Sprintf("msg-%d", timeNow().UnixNano())
	msgPath := filepath.Join(intermuteDir, msgID+".yaml")

	msg := map[string]interface{}{
		"id":      msgID,
		"from":    "mcp-agent",
		"to":      to,
		"subject": subject,
		"body":    body,
		"sent_at": timeNow().Format("2006-01-02T15:04:05Z07:00"),
	}

	data, err := yaml.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}

	if err := os.WriteFile(msgPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write message: %w", err)
	}

	return map[string]interface{}{
		"success":    true,
		"message_id": msgID,
		"to":         to,
	}, nil
}

// Helper functions

func readPRDSummary(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var prd map[string]interface{}
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return nil, err
	}

	// Return summary fields only
	return map[string]interface{}{
		"id":     prd["id"],
		"title":  prd["title"],
		"status": prd["status"],
	}, nil
}

func readTaskSummary(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var task map[string]interface{}
	if err := yaml.Unmarshal(data, &task); err != nil {
		return nil, err
	}

	// Return summary fields only
	return map[string]interface{}{
		"id":       task["id"],
		"title":    task["title"],
		"status":   task["status"],
		"prd_id":   task["prd_id"],
		"assignee": task["assignee"],
	}, nil
}

func countFilesByStatus(dir, statusField string) map[string]int {
	counts := make(map[string]int)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return counts
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var doc map[string]interface{}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}

		if status, ok := doc[statusField].(string); ok {
			counts[status]++
		}
	}

	return counts
}

func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			count++
		}
	}
	return count
}

func sumValues(m map[string]int) int {
	sum := 0
	for _, v := range m {
		sum += v
	}
	return sum
}

func containsAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// timeNow is a variable for testing
var timeNow = time.Now
