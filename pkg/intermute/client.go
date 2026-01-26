// Package intermute provides a unified client for Autarch tools to communicate
// with the Intermute coordination server. It wraps the base Intermute client
// with Autarch-specific conveniences and integration patterns.
package intermute

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	ic "github.com/mistakeknot/intermute/client"
)

// Client provides a unified interface to the Intermute coordination server
// for all Autarch tools (Gurgeh, Coldwine, Pollard, Bigend).
type Client struct {
	base      *ic.Client
	ws        *ic.WSClient
	project   string
	agentID   string
	agentName string

	mu       sync.RWMutex
	handlers map[string][]EventHandler
}

// EventHandler processes domain events
type EventHandler func(Event)

// Event wraps an Intermute domain event with Autarch-specific typing
type Event struct {
	Type      string
	Project   string
	EntityID  string
	Data      any
	Timestamp time.Time
}

// Config holds configuration for the Intermute client
type Config struct {
	URL       string // Intermute server URL (default: from INTERMUTE_URL env)
	APIKey    string // API key for authentication (default: from INTERMUTE_API_KEY env)
	Project   string // Project scope (default: from INTERMUTE_PROJECT env)
	AgentName string // Agent name for registration
	AgentID   string // Pre-existing agent ID (skip registration if set)
}

// NewClient creates a new Intermute client with the given configuration.
// If config is nil, it uses environment variables for configuration.
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	// Load from environment if not specified
	if cfg.URL == "" {
		cfg.URL = strings.TrimSpace(os.Getenv("INTERMUTE_URL"))
	}
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(os.Getenv("INTERMUTE_API_KEY"))
	}
	if cfg.Project == "" {
		cfg.Project = strings.TrimSpace(os.Getenv("INTERMUTE_PROJECT"))
	}

	if cfg.URL == "" {
		return nil, fmt.Errorf("INTERMUTE_URL required: set environment variable or config.URL")
	}

	var clientOpts []ic.Option
	if cfg.APIKey != "" {
		clientOpts = append(clientOpts, ic.WithAPIKey(cfg.APIKey))
	}
	if cfg.Project != "" {
		clientOpts = append(clientOpts, ic.WithProject(cfg.Project))
	}

	base := ic.New(cfg.URL, clientOpts...)

	c := &Client{
		base:      base,
		project:   cfg.Project,
		agentName: cfg.AgentName,
		agentID:   cfg.AgentID,
		handlers:  make(map[string][]EventHandler),
	}

	return c, nil
}

// Connect establishes WebSocket connection for real-time events.
// Call this after NewClient if you need real-time event subscriptions.
func (c *Client) Connect(ctx context.Context) error {
	var wsOpts []ic.WSOption
	if c.project != "" {
		wsOpts = append(wsOpts, ic.WithWSProject(c.project))
	}
	if c.agentID != "" {
		wsOpts = append(wsOpts, ic.WithWSAgentID(c.agentID))
	}
	wsOpts = append(wsOpts, ic.WithAutoReconnect(true))

	c.ws = ic.NewWSClient(c.base.BaseURL, wsOpts...)

	// Wire up event dispatching
	c.ws.OnEvent(func(evt ic.DomainEvent) {
		c.dispatchEvent(Event{
			Type:      evt.Type,
			Project:   evt.Project,
			EntityID:  evt.EntityID,
			Data:      evt.Data,
			Timestamp: evt.CreatedAt,
		})
	})

	return c.ws.Connect(ctx)
}

// Close closes the client connections
func (c *Client) Close() error {
	if c.ws != nil {
		return c.ws.Close()
	}
	return nil
}

// On registers an event handler for specific event types.
// Pass "*" to receive all events.
func (c *Client) On(eventType string, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[eventType] = append(c.handlers[eventType], handler)
}

// Subscribe subscribes to specific event types via WebSocket
func (c *Client) Subscribe(ctx context.Context, eventTypes ...string) error {
	if c.ws == nil {
		return fmt.Errorf("not connected: call Connect first")
	}
	return c.ws.Subscribe(ctx, eventTypes...)
}

func (c *Client) dispatchEvent(evt Event) {
	c.mu.RLock()
	// Get handlers for this specific event type
	handlers := make([]EventHandler, 0)
	handlers = append(handlers, c.handlers[evt.Type]...)
	// Get handlers for wildcard
	handlers = append(handlers, c.handlers["*"]...)
	c.mu.RUnlock()

	for _, h := range handlers {
		h(evt)
	}
}

// --- Spec Operations ---

// CreateSpec creates a new specification in Intermute
func (c *Client) CreateSpec(ctx context.Context, spec Spec) (Spec, error) {
	created, err := c.base.CreateSpec(ctx, toIntermuteSpec(spec))
	if err != nil {
		return Spec{}, err
	}
	return fromIntermuteSpec(created), nil
}

// GetSpec retrieves a specification by ID
func (c *Client) GetSpec(ctx context.Context, id string) (Spec, error) {
	spec, err := c.base.GetSpec(ctx, id)
	if err != nil {
		return Spec{}, err
	}
	return fromIntermuteSpec(spec), nil
}

// ListSpecs lists specifications with optional status filter
func (c *Client) ListSpecs(ctx context.Context, status string) ([]Spec, error) {
	specs, err := c.base.ListSpecs(ctx, status)
	if err != nil {
		return nil, err
	}
	result := make([]Spec, len(specs))
	for i, s := range specs {
		result[i] = fromIntermuteSpec(s)
	}
	return result, nil
}

// UpdateSpec updates a specification
func (c *Client) UpdateSpec(ctx context.Context, spec Spec) (Spec, error) {
	updated, err := c.base.UpdateSpec(ctx, toIntermuteSpec(spec))
	if err != nil {
		return Spec{}, err
	}
	return fromIntermuteSpec(updated), nil
}

// DeleteSpec deletes a specification
func (c *Client) DeleteSpec(ctx context.Context, id string) error {
	return c.base.DeleteSpec(ctx, id)
}

// --- Epic Operations ---

// CreateEpic creates a new epic in Intermute
func (c *Client) CreateEpic(ctx context.Context, epic Epic) (Epic, error) {
	created, err := c.base.CreateEpic(ctx, toIntermuteEpic(epic))
	if err != nil {
		return Epic{}, err
	}
	return fromIntermuteEpic(created), nil
}

// GetEpic retrieves an epic by ID
func (c *Client) GetEpic(ctx context.Context, id string) (Epic, error) {
	epic, err := c.base.GetEpic(ctx, id)
	if err != nil {
		return Epic{}, err
	}
	return fromIntermuteEpic(epic), nil
}

// ListEpics lists epics with optional spec filter
func (c *Client) ListEpics(ctx context.Context, specID string) ([]Epic, error) {
	epics, err := c.base.ListEpics(ctx, specID)
	if err != nil {
		return nil, err
	}
	result := make([]Epic, len(epics))
	for i, e := range epics {
		result[i] = fromIntermuteEpic(e)
	}
	return result, nil
}

// UpdateEpic updates an epic
func (c *Client) UpdateEpic(ctx context.Context, epic Epic) (Epic, error) {
	updated, err := c.base.UpdateEpic(ctx, toIntermuteEpic(epic))
	if err != nil {
		return Epic{}, err
	}
	return fromIntermuteEpic(updated), nil
}

// DeleteEpic deletes an epic
func (c *Client) DeleteEpic(ctx context.Context, id string) error {
	return c.base.DeleteEpic(ctx, id)
}

// --- Story Operations ---

// CreateStory creates a new story in Intermute
func (c *Client) CreateStory(ctx context.Context, story Story) (Story, error) {
	created, err := c.base.CreateStory(ctx, toIntermuteStory(story))
	if err != nil {
		return Story{}, err
	}
	return fromIntermuteStory(created), nil
}

// GetStory retrieves a story by ID
func (c *Client) GetStory(ctx context.Context, id string) (Story, error) {
	story, err := c.base.GetStory(ctx, id)
	if err != nil {
		return Story{}, err
	}
	return fromIntermuteStory(story), nil
}

// ListStories lists stories with optional epic filter
func (c *Client) ListStories(ctx context.Context, epicID string) ([]Story, error) {
	stories, err := c.base.ListStories(ctx, epicID)
	if err != nil {
		return nil, err
	}
	result := make([]Story, len(stories))
	for i, s := range stories {
		result[i] = fromIntermuteStory(s)
	}
	return result, nil
}

// UpdateStory updates a story
func (c *Client) UpdateStory(ctx context.Context, story Story) (Story, error) {
	updated, err := c.base.UpdateStory(ctx, toIntermuteStory(story))
	if err != nil {
		return Story{}, err
	}
	return fromIntermuteStory(updated), nil
}

// DeleteStory deletes a story
func (c *Client) DeleteStory(ctx context.Context, id string) error {
	return c.base.DeleteStory(ctx, id)
}

// --- Task Operations ---

// CreateTask creates a new task in Intermute
func (c *Client) CreateTask(ctx context.Context, task Task) (Task, error) {
	created, err := c.base.CreateTask(ctx, toIntermuteTask(task))
	if err != nil {
		return Task{}, err
	}
	return fromIntermuteTask(created), nil
}

// GetTask retrieves a task by ID
func (c *Client) GetTask(ctx context.Context, id string) (Task, error) {
	task, err := c.base.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return fromIntermuteTask(task), nil
}

// ListTasks lists tasks with optional filters
func (c *Client) ListTasks(ctx context.Context, status, agent string) ([]Task, error) {
	tasks, err := c.base.ListTasks(ctx, status, agent)
	if err != nil {
		return nil, err
	}
	result := make([]Task, len(tasks))
	for i, t := range tasks {
		result[i] = fromIntermuteTask(t)
	}
	return result, nil
}

// UpdateTask updates a task
func (c *Client) UpdateTask(ctx context.Context, task Task) (Task, error) {
	updated, err := c.base.UpdateTask(ctx, toIntermuteTask(task))
	if err != nil {
		return Task{}, err
	}
	return fromIntermuteTask(updated), nil
}

// AssignTask assigns a task to an agent
func (c *Client) AssignTask(ctx context.Context, taskID, agent string) (Task, error) {
	assigned, err := c.base.AssignTask(ctx, taskID, agent)
	if err != nil {
		return Task{}, err
	}
	return fromIntermuteTask(assigned), nil
}

// DeleteTask deletes a task
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	return c.base.DeleteTask(ctx, id)
}

// --- Insight Operations ---

// CreateInsight creates a new insight in Intermute
func (c *Client) CreateInsight(ctx context.Context, insight Insight) (Insight, error) {
	created, err := c.base.CreateInsight(ctx, toIntermuteInsight(insight))
	if err != nil {
		return Insight{}, err
	}
	return fromIntermuteInsight(created), nil
}

// GetInsight retrieves an insight by ID
func (c *Client) GetInsight(ctx context.Context, id string) (Insight, error) {
	insight, err := c.base.GetInsight(ctx, id)
	if err != nil {
		return Insight{}, err
	}
	return fromIntermuteInsight(insight), nil
}

// ListInsights lists insights with optional filters
func (c *Client) ListInsights(ctx context.Context, specID, category string) ([]Insight, error) {
	insights, err := c.base.ListInsights(ctx, specID, category)
	if err != nil {
		return nil, err
	}
	result := make([]Insight, len(insights))
	for i, in := range insights {
		result[i] = fromIntermuteInsight(in)
	}
	return result, nil
}

// LinkInsightToSpec links an insight to a specification
func (c *Client) LinkInsightToSpec(ctx context.Context, insightID, specID string) error {
	return c.base.LinkInsightToSpec(ctx, insightID, specID)
}

// DeleteInsight deletes an insight
func (c *Client) DeleteInsight(ctx context.Context, id string) error {
	return c.base.DeleteInsight(ctx, id)
}

// --- Session Operations ---

// CreateSession creates a new agent session in Intermute
func (c *Client) CreateSession(ctx context.Context, session Session) (Session, error) {
	created, err := c.base.CreateSession(ctx, toIntermuteSession(session))
	if err != nil {
		return Session{}, err
	}
	return fromIntermuteSession(created), nil
}

// GetSession retrieves a session by ID
func (c *Client) GetSession(ctx context.Context, id string) (Session, error) {
	session, err := c.base.GetSession(ctx, id)
	if err != nil {
		return Session{}, err
	}
	return fromIntermuteSession(session), nil
}

// ListSessions lists sessions with optional status filter
func (c *Client) ListSessions(ctx context.Context, status string) ([]Session, error) {
	sessions, err := c.base.ListSessions(ctx, status)
	if err != nil {
		return nil, err
	}
	result := make([]Session, len(sessions))
	for i, s := range sessions {
		result[i] = fromIntermuteSession(s)
	}
	return result, nil
}

// UpdateSession updates a session
func (c *Client) UpdateSession(ctx context.Context, session Session) (Session, error) {
	updated, err := c.base.UpdateSession(ctx, toIntermuteSession(session))
	if err != nil {
		return Session{}, err
	}
	return fromIntermuteSession(updated), nil
}

// DeleteSession deletes a session
func (c *Client) DeleteSession(ctx context.Context, id string) error {
	return c.base.DeleteSession(ctx, id)
}

// --- CUJ Operations ---

// CreateCUJ creates a new Critical User Journey in Intermute
func (c *Client) CreateCUJ(ctx context.Context, cuj CriticalUserJourney) (CriticalUserJourney, error) {
	created, err := c.base.CreateCUJ(ctx, toIntermuteCUJ(cuj))
	if err != nil {
		return CriticalUserJourney{}, err
	}
	return fromIntermuteCUJ(created), nil
}

// GetCUJ retrieves a CUJ by ID
func (c *Client) GetCUJ(ctx context.Context, id string) (CriticalUserJourney, error) {
	cuj, err := c.base.GetCUJ(ctx, id)
	if err != nil {
		return CriticalUserJourney{}, err
	}
	return fromIntermuteCUJ(cuj), nil
}

// ListCUJs lists CUJs with optional spec filter
func (c *Client) ListCUJs(ctx context.Context, specID string) ([]CriticalUserJourney, error) {
	cujs, err := c.base.ListCUJs(ctx, specID)
	if err != nil {
		return nil, err
	}
	result := make([]CriticalUserJourney, len(cujs))
	for i, cuj := range cujs {
		result[i] = fromIntermuteCUJ(cuj)
	}
	return result, nil
}

// UpdateCUJ updates a CUJ
func (c *Client) UpdateCUJ(ctx context.Context, cuj CriticalUserJourney) (CriticalUserJourney, error) {
	updated, err := c.base.UpdateCUJ(ctx, toIntermuteCUJ(cuj))
	if err != nil {
		return CriticalUserJourney{}, err
	}
	return fromIntermuteCUJ(updated), nil
}

// DeleteCUJ deletes a CUJ
func (c *Client) DeleteCUJ(ctx context.Context, id string) error {
	return c.base.DeleteCUJ(ctx, id)
}

// LinkCUJToFeature links a CUJ to a feature
func (c *Client) LinkCUJToFeature(ctx context.Context, cujID, featureID string) error {
	return c.base.LinkCUJToFeature(ctx, cujID, featureID)
}

// UnlinkCUJFromFeature removes a link between a CUJ and a feature
func (c *Client) UnlinkCUJFromFeature(ctx context.Context, cujID, featureID string) error {
	return c.base.UnlinkCUJFromFeature(ctx, cujID, featureID)
}

// GetCUJFeatureLinks gets all feature links for a CUJ
func (c *Client) GetCUJFeatureLinks(ctx context.Context, cujID string) ([]CUJFeatureLink, error) {
	links, err := c.base.GetCUJFeatureLinks(ctx, cujID)
	if err != nil {
		return nil, err
	}
	result := make([]CUJFeatureLink, len(links))
	for i, link := range links {
		result[i] = CUJFeatureLink{
			CUJID:     link.CUJID,
			FeatureID: link.FeatureID,
			Project:   link.Project,
			LinkedAt:  link.LinkedAt,
		}
	}
	return result, nil
}

// --- Messaging Operations (delegate to base client) ---

// SendMessage sends a message via Intermute
func (c *Client) SendMessage(ctx context.Context, msg ic.Message) (ic.SendResponse, error) {
	return c.base.SendMessage(ctx, msg)
}

// InboxSince retrieves messages since a cursor
func (c *Client) InboxSince(ctx context.Context, agent string, cursor uint64) (ic.InboxResponse, error) {
	return c.base.InboxSince(ctx, agent, cursor)
}

// ListAgents lists registered agents
func (c *Client) ListAgents(ctx context.Context, project string) ([]ic.Agent, error) {
	return c.base.ListAgents(ctx, project)
}

// EventTypes provides standard domain event type constants
var EventTypes = ic.EventTypes

// ErrConflict is returned when optimistic locking fails
var ErrConflict = ic.ErrConflict

// --- Agent Operations (with inbox enrichment) ---

// GetAgent retrieves an agent by name and enriches with inbox counts
func (c *Client) GetAgent(ctx context.Context, name string) (*Agent, error) {
	agents, err := c.base.ListAgents(ctx, c.project)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a.Name == name || a.ID == name {
			agent := fromIntermuteAgent(a)
			// Enrich with inbox counts
			counts, err := c.base.InboxCounts(ctx, a.ID)
			if err == nil {
				agent.InboxCount = counts.Total
				agent.UnreadCount = counts.Unread
			}
			return &agent, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", name)
}

// ListAgentsEnriched lists all agents with inbox counts
func (c *Client) ListAgentsEnriched(ctx context.Context) ([]Agent, error) {
	agents, err := c.base.ListAgents(ctx, c.project)
	if err != nil {
		return nil, err
	}
	result := make([]Agent, len(agents))
	for i, a := range agents {
		result[i] = fromIntermuteAgent(a)
		// Enrich with inbox counts
		counts, err := c.base.InboxCounts(ctx, a.ID)
		if err == nil {
			result[i].InboxCount = counts.Total
			result[i].UnreadCount = counts.Unread
		}
	}
	return result, nil
}

// --- Inbox Operations ---

// InboxCounts returns total and unread message counts for an agent
func (c *Client) InboxCounts(ctx context.Context, agentID string) (InboxCounts, error) {
	counts, err := c.base.InboxCounts(ctx, agentID)
	if err != nil {
		return InboxCounts{}, err
	}
	return InboxCounts{Total: counts.Total, Unread: counts.Unread}, nil
}

// AgentMessages retrieves messages for an agent (uses InboxSince with cursor 0)
func (c *Client) AgentMessages(ctx context.Context, agentID string, limit int) ([]Message, error) {
	resp, err := c.base.InboxSince(ctx, agentID, 0)
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, 0, len(resp.Messages))
	for i, m := range resp.Messages {
		if limit > 0 && i >= limit {
			break
		}
		msgs = append(msgs, fromIntermuteMessage(m))
	}
	return msgs, nil
}

// --- Reservation Operations ---

// Reserve creates a new file reservation
func (c *Client) Reserve(ctx context.Context, r Reservation, ttlMinutes int) (*Reservation, error) {
	if r.Project == "" {
		r.Project = c.project
	}
	created, err := c.base.Reserve(ctx, toIntermuteReservation(r, ttlMinutes))
	if err != nil {
		return nil, err
	}
	result := fromIntermuteReservation(created)
	return &result, nil
}

// ReleaseReservation releases a file reservation by ID
func (c *Client) ReleaseReservation(ctx context.Context, id string) error {
	return c.base.ReleaseReservation(ctx, id)
}

// ActiveReservations returns all active reservations for the project
func (c *Client) ActiveReservations(ctx context.Context) ([]Reservation, error) {
	reservations, err := c.base.ActiveReservations(ctx, c.project)
	if err != nil {
		return nil, err
	}
	result := make([]Reservation, len(reservations))
	for i, r := range reservations {
		result[i] = fromIntermuteReservation(r)
	}
	return result, nil
}

// AgentReservations returns all reservations held by an agent
func (c *Client) AgentReservations(ctx context.Context, agentID string) ([]Reservation, error) {
	reservations, err := c.base.AgentReservations(ctx, agentID)
	if err != nil {
		return nil, err
	}
	result := make([]Reservation, len(reservations))
	for i, r := range reservations {
		result[i] = fromIntermuteReservation(r)
	}
	return result, nil
}
