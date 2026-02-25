package autarch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

// Client provides typed access to the Intermute domain API.
// When a fallback DataSource is configured and a dial error occurs,
// the client switches to the fallback for the remainder of the session.
type Client struct {
	baseURL    string
	httpClient *http.Client
	project    string

	fallback       DataSource
	fallbackActive atomic.Bool
}

// NewClient creates a new Intermute domain client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithProject sets a default project for all requests
func (c *Client) WithProject(project string) *Client {
	c.project = project
	return c
}

// WithFallback configures a local DataSource to use when Intermute is unreachable.
// Call this before distributing the client to views.
func (c *Client) WithFallback(ds DataSource) *Client {
	c.fallback = ds
	return c
}

// InFallbackMode reports whether the client has switched to local data.
// Views check this to display an offline indicator.
func (c *Client) InFallbackMode() bool {
	return c.fallbackActive.Load()
}

// isDialError returns true if the error indicates a TCP dial failure
// (connection refused). Does NOT match timeouts — the server may be alive
// but slow, and switching to stale local data would be worse.
func isDialError(err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}
	if opErr.Op != "dial" {
		return false
	}
	// Check for ECONNREFUSED specifically
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		return errors.Is(sysErr.Err, syscall.ECONNREFUSED)
	}
	return false
}

// tryFallback checks if an error is a dial error and switches to fallback.
// Returns true if fallback was activated (caller should retry via fallback).
func (c *Client) tryFallback(err error) bool {
	if c.fallback == nil || c.fallbackActive.Load() {
		return false
	}
	if isDialError(err) {
		c.fallbackActive.Store(true)
		return true
	}
	return false
}

// ErrFallbackReadOnly is returned when a write operation is attempted while
// the client is in fallback mode (Intermute unreachable). Start Intermute
// to enable writes: autarch tui
var ErrFallbackReadOnly = errors.New("Intermute is not running — writes unavailable. Start with: autarch tui")

// checkWritable returns ErrFallbackReadOnly if the client is in fallback mode.
func (c *Client) checkWritable() error {
	if c.fallbackActive.Load() && c.fallback != nil {
		return ErrFallbackReadOnly
	}
	return nil
}

// request helpers

func (c *Client) get(path string, query url.Values, result any) error {
	u := c.baseURL + path
	if query != nil {
		if c.project != "" && query.Get("project") == "" {
			query.Set("project", c.project)
		}
		u += "?" + query.Encode()
	} else if c.project != "" {
		u += "?project=" + url.QueryEscape(c.project)
	}

	resp, err := c.httpClient.Get(u)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) post(path string, body, result any) error {
	u := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	resp, err := c.httpClient.Post(u, "application/json", reqBody)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, string(respBody))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) put(path string, body, result any) error {
	u := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPut, u, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PUT %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT %s: %d %s", path, resp.StatusCode, string(respBody))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) delete(path string) error {
	u := c.baseURL + path
	if c.project != "" {
		u += "?project=" + url.QueryEscape(c.project)
	}

	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: %d %s", path, resp.StatusCode, string(body))
	}
	return nil
}

// Spec operations

func (c *Client) CreateSpec(spec Spec) (Spec, error) {
	if err := c.checkWritable(); err != nil {
		return Spec{}, err
	}
	if spec.Project == "" {
		spec.Project = c.project
	}
	var result Spec
	if err := c.post("/api/specs", spec, &result); err != nil {
		return Spec{}, err
	}
	return result, nil
}

func (c *Client) GetSpec(id string) (Spec, error) {
	var result Spec
	if err := c.get("/api/specs/"+id, nil, &result); err != nil {
		return Spec{}, err
	}
	return result, nil
}

func (c *Client) ListSpecs(status string) ([]Spec, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		return c.fallback.ListSpecs(status)
	}
	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	var result []Spec
	if err := c.get("/api/specs", query, &result); err != nil {
		if c.tryFallback(err) {
			return c.fallback.ListSpecs(status)
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) UpdateSpec(spec Spec) (Spec, error) {
	if err := c.checkWritable(); err != nil {
		return Spec{}, err
	}
	var result Spec
	if err := c.put("/api/specs/"+spec.ID, spec, &result); err != nil {
		return Spec{}, err
	}
	return result, nil
}

func (c *Client) DeleteSpec(id string) error {
	if err := c.checkWritable(); err != nil {
		return err
	}
	return c.delete("/api/specs/" + id)
}

// Epic operations

func (c *Client) CreateEpic(epic Epic) (Epic, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		if w, ok := c.fallback.(WritableDataSource); ok {
			if epic.Project == "" {
				epic.Project = c.project
			}
			return w.CreateEpic(epic)
		}
		return Epic{}, ErrFallbackReadOnly
	}
	if epic.Project == "" {
		epic.Project = c.project
	}
	var result Epic
	if err := c.post("/api/epics", epic, &result); err != nil {
		return Epic{}, err
	}
	return result, nil
}

func (c *Client) GetEpic(id string) (Epic, error) {
	var result Epic
	if err := c.get("/api/epics/"+id, nil, &result); err != nil {
		return Epic{}, err
	}
	return result, nil
}

func (c *Client) ListEpics(specID string) ([]Epic, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		return c.fallback.ListEpics(specID)
	}
	query := url.Values{}
	if specID != "" {
		query.Set("spec", specID)
	}
	var result []Epic
	if err := c.get("/api/epics", query, &result); err != nil {
		if c.tryFallback(err) {
			return c.fallback.ListEpics(specID)
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) UpdateEpic(epic Epic) (Epic, error) {
	if err := c.checkWritable(); err != nil {
		return Epic{}, err
	}
	var result Epic
	if err := c.put("/api/epics/"+epic.ID, epic, &result); err != nil {
		return Epic{}, err
	}
	return result, nil
}

func (c *Client) DeleteEpic(id string) error {
	if err := c.checkWritable(); err != nil {
		return err
	}
	return c.delete("/api/epics/" + id)
}

// Story operations

func (c *Client) CreateStory(story Story) (Story, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		if w, ok := c.fallback.(WritableDataSource); ok {
			if story.Project == "" {
				story.Project = c.project
			}
			return w.CreateStory(story)
		}
		return Story{}, ErrFallbackReadOnly
	}
	if story.Project == "" {
		story.Project = c.project
	}
	var result Story
	if err := c.post("/api/stories", story, &result); err != nil {
		return Story{}, err
	}
	return result, nil
}

func (c *Client) GetStory(id string) (Story, error) {
	var result Story
	if err := c.get("/api/stories/"+id, nil, &result); err != nil {
		return Story{}, err
	}
	return result, nil
}

func (c *Client) ListStories(epicID string) ([]Story, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		return c.fallback.ListStories(epicID)
	}
	query := url.Values{}
	if epicID != "" {
		query.Set("epic", epicID)
	}
	var result []Story
	if err := c.get("/api/stories", query, &result); err != nil {
		if c.tryFallback(err) {
			return c.fallback.ListStories(epicID)
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) UpdateStory(story Story) (Story, error) {
	if err := c.checkWritable(); err != nil {
		return Story{}, err
	}
	var result Story
	if err := c.put("/api/stories/"+story.ID, story, &result); err != nil {
		return Story{}, err
	}
	return result, nil
}

func (c *Client) DeleteStory(id string) error {
	if err := c.checkWritable(); err != nil {
		return err
	}
	return c.delete("/api/stories/" + id)
}

// Task operations

func (c *Client) CreateTask(task Task) (Task, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		if w, ok := c.fallback.(WritableDataSource); ok {
			if task.Project == "" {
				task.Project = c.project
			}
			return w.CreateTask(task)
		}
		return Task{}, ErrFallbackReadOnly
	}
	if task.Project == "" {
		task.Project = c.project
	}
	var result Task
	if err := c.post("/api/tasks", task, &result); err != nil {
		return Task{}, err
	}
	return result, nil
}

func (c *Client) GetTask(id string) (Task, error) {
	var result Task
	if err := c.get("/api/tasks/"+id, nil, &result); err != nil {
		return Task{}, err
	}
	return result, nil
}

func (c *Client) ListTasks(status, agent string) ([]Task, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		return c.fallback.ListTasks(status, agent)
	}
	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	if agent != "" {
		query.Set("agent", agent)
	}
	var result []Task
	if err := c.get("/api/tasks", query, &result); err != nil {
		if c.tryFallback(err) {
			return c.fallback.ListTasks(status, agent)
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) UpdateTask(task Task) (Task, error) {
	if err := c.checkWritable(); err != nil {
		return Task{}, err
	}
	var result Task
	if err := c.put("/api/tasks/"+task.ID, task, &result); err != nil {
		return Task{}, err
	}
	return result, nil
}

func (c *Client) AssignTask(id, agent string) (Task, error) {
	if err := c.checkWritable(); err != nil {
		return Task{}, err
	}
	var result Task
	if err := c.post("/api/tasks/"+id+"/assign", map[string]string{"agent": agent}, &result); err != nil {
		return Task{}, err
	}
	return result, nil
}

func (c *Client) DeleteTask(id string) error {
	if err := c.checkWritable(); err != nil {
		return err
	}
	return c.delete("/api/tasks/" + id)
}

// Insight operations

func (c *Client) CreateInsight(insight Insight) (Insight, error) {
	if err := c.checkWritable(); err != nil {
		return Insight{}, err
	}
	if insight.Project == "" {
		insight.Project = c.project
	}
	var result Insight
	if err := c.post("/api/insights", insight, &result); err != nil {
		return Insight{}, err
	}
	return result, nil
}

func (c *Client) GetInsight(id string) (Insight, error) {
	var result Insight
	if err := c.get("/api/insights/"+id, nil, &result); err != nil {
		return Insight{}, err
	}
	return result, nil
}

func (c *Client) ListInsights(specID, category string) ([]Insight, error) {
	if c.fallbackActive.Load() && c.fallback != nil {
		return c.fallback.ListInsights(specID, category)
	}
	query := url.Values{}
	if specID != "" {
		query.Set("spec", specID)
	}
	if category != "" {
		query.Set("category", category)
	}
	var result []Insight
	if err := c.get("/api/insights", query, &result); err != nil {
		if c.tryFallback(err) {
			return c.fallback.ListInsights(specID, category)
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) LinkInsight(id, specID string) error {
	if err := c.checkWritable(); err != nil {
		return err
	}
	return c.post("/api/insights/"+id+"/link", map[string]string{"spec_id": specID}, nil)
}

func (c *Client) DeleteInsight(id string) error {
	if err := c.checkWritable(); err != nil {
		return err
	}
	return c.delete("/api/insights/" + id)
}

// Session operations

func (c *Client) CreateSession(session Session) (Session, error) {
	if err := c.checkWritable(); err != nil {
		return Session{}, err
	}
	if session.Project == "" {
		session.Project = c.project
	}
	var result Session
	if err := c.post("/api/sessions", session, &result); err != nil {
		return Session{}, err
	}
	return result, nil
}

func (c *Client) GetSession(id string) (Session, error) {
	var result Session
	if err := c.get("/api/sessions/"+id, nil, &result); err != nil {
		return Session{}, err
	}
	return result, nil
}

func (c *Client) ListSessions(status string) ([]Session, error) {
	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	var result []Session
	if err := c.get("/api/sessions", query, &result); err != nil {
		if c.tryFallback(err) {
			// Sessions are infrastructure data, not in DataSource.
			// Return empty list so the dashboard renders in degraded mode.
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) UpdateSession(session Session) (Session, error) {
	if err := c.checkWritable(); err != nil {
		return Session{}, err
	}
	var result Session
	if err := c.put("/api/sessions/"+session.ID, session, &result); err != nil {
		return Session{}, err
	}
	return result, nil
}

func (c *Client) DeleteSession(id string) error {
	if err := c.checkWritable(); err != nil {
		return err
	}
	return c.delete("/api/sessions/" + id)
}
