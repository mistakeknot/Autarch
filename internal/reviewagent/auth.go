package reviewagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/review"
)

func credentialProfile(project, authPath, lockPath string) (string, error) {
	if !filepath.IsAbs(authPath) || lockPath != authPath+".lock" {
		return "", errors.New("invalid credential storage paths")
	}
	for _, path := range []string{authPath, lockPath} {
		rel, err := filepath.Rel(project, path)
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return "", errors.New("credential storage must be outside the project")
		}
	}
	return fmt.Sprintf("(allow file-write* (literal %s) (subpath %s))\n", strconv.Quote(authPath), strconv.Quote(lockPath)), nil
}

func prepareCredentials(binary, project string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--auth-storage-prepare", "--deny-project", project)
	cmd.WaitDelay = 500 * time.Millisecond
	// Resolve the provider's credential store through trusted entry code, before
	// the investigation sandbox. Never copy credentials from other applications.
	cmd.Dir = "/"
	data, err := cmd.Output()
	var paths struct {
		AuthPath string `json:"authPath"`
		LockPath string `json:"lockPath"`
	}
	if err != nil || json.Unmarshal(data, &paths) != nil {
		return "", errors.New("credential storage initialization failed; check Flere shared storage permissions")
	}
	return credentialProfile(project, paths.AuthPath, paths.LockPath)
}

func (e *Engine) Auth(r review.Request) review.Response {
	response := review.Response{Version: review.Version, ID: r.ID}
	if r.Method == "auth.display" {
		e.mu.Lock()
		c := e.authDisplay
		displayID := e.authDisplayID
		e.mu.Unlock()
		if c != nil {
			c.mu.Lock()
			if !c.dead && c.auth != nil {
				data, _ := json.Marshal(c.auth)
				_ = json.Unmarshal(data, &response.Auth)
				response.Auth.DisplayID = displayID
			}
			c.mu.Unlock()
		}
		return response
	}
	commands := map[string]string{"auth.providers": "get_providers", "auth.status": "get_auth_status", "auth.login": "auth_login", "auth.respond": "auth_response", "auth.cancel": "auth_cancel", "auth.logout": "auth_logout"}
	command, ok := commands[r.Method]
	if !ok {
		response.Error = "unknown authentication action"
		return response
	}
	project, err := filepath.EvalSymlinks(r.Project)
	if err != nil || !filepath.IsAbs(project) {
		response.Error = "select an existing project"
		return response
	}
	var c *conversation
	if r.Method == "auth.providers" {
		c, err = e.start(project)
	} else {
		e.mu.Lock()
		c = e.runtimes[project]
		e.mu.Unlock()
	}
	if err != nil {
		response.Error = err.Error()
		return response
	}
	if c == nil {
		response.Error = "Flere disconnected; reconnect the provider panel"
		return response
	}
	if r.Method == "auth.providers" || r.Method == "auth.login" {
		e.mu.Lock()
		e.authDisplay = c
		e.authDisplayID = review.NewID()
		e.mu.Unlock()
	}
	request := map[string]any{"type": command, "id": "auth-" + review.NewID()}
	if r.Auth != nil {
		request["runtimeId"], request["operationId"], request["promptId"] = r.Auth.RuntimeID, r.Auth.OperationID, r.Auth.PromptID
		request["provider"], request["authType"], request["value"] = r.Auth.Provider, r.Auth.AuthType, r.Auth.Value
	}
	id := request["id"].(string)
	waiter := make(chan review.Response, 1)
	c.mu.Lock()
	if c.authWaiters == nil {
		c.authWaiters = map[string]chan review.Response{}
	}
	c.authWaiters[id] = waiter
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.authWaiters, id); c.mu.Unlock() }()
	if err = c.send(request); err != nil {
		response.Error = "Flere disconnected; reconnect the provider panel"
		return response
	}
	select {
	case result := <-waiter:
		result.ID = r.ID
		return result
	case <-time.After(8 * time.Second):
		response.Error = "Provider connection did not respond; reopen Connect provider to recover its status"
		return response
	}
}

// Called before the durable conversation event handler. Raw provider errors,
// prompts, codes and callback URLs stay in memory even when the operation fails.
func (e *Engine) authEvent(c *conversation, event map[string]json.RawMessage) bool {
	typeName, command := stringField(event, "type"), stringField(event, "command")
	if typeName != "auth_state" && !(typeName == "response" && (strings.HasPrefix(command, "auth_") || command == "get_auth_status" || command == "get_providers")) {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	response := review.Response{Version: review.Version}
	var success bool
	_ = json.Unmarshal(event["success"], &success)
	if typeName == "auth_state" || success {
		var state review.AuthState
		if json.Unmarshal(event["data"], &state) == nil {
			state.Project = c.project
			c.auth = &state
			response.Auth = &state
			if state.Model != nil {
				model := state.Model.Provider + "/" + state.Model.ID
				if model != c.model {
					c.model = model
					e.record(c.project, "handoff", "Provider catalog selected "+model+" for the Flere conversation. Clavain execution routing remains governed independently.", c.session, model)
				}
			}
		}
	} else {
		response.Error = "Provider connection request failed or is stale; refresh Connect provider"
		if stringField(event, "errorCode") == "storage_error" {
			response.Error = "Shared credential storage failed; check Flere storage permissions"
		}
	}
	if waiter := c.authWaiters[stringField(event, "id")]; waiter != nil {
		select {
		case waiter <- response:
		default:
		}
	}
	return true
}
