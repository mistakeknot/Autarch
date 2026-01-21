package agentmail

import "time"

// Agent represents a registered AI agent
type Agent struct {
	ID               int       `json:"id"`
	ProjectID        int       `json:"project_id"`
	Name             string    `json:"name"`
	Program          string    `json:"program"`
	Model            string    `json:"model"`
	TaskDescription  string    `json:"task_description"`
	InceptionTS      time.Time `json:"inception_ts"`
	LastActiveTS     time.Time `json:"last_active_ts"`
	AttachmentsPolicy string   `json:"attachments_policy"`
	ContactPolicy    string    `json:"contact_policy"`
	// Computed fields
	ProjectPath      string    `json:"project_path,omitempty"`
	InboxCount       int       `json:"inbox_count"`
	UnreadCount      int       `json:"unread_count"`
}

// Project represents an MCP Agent Mail project
type Project struct {
	ID        int       `json:"id"`
	Slug      string    `json:"slug"`
	HumanKey  string    `json:"human_key"` // Usually the absolute path
	CreatedAt time.Time `json:"created_at"`
}

// Message represents a message between agents
type Message struct {
	ID          int       `json:"id"`
	ProjectID   int       `json:"project_id"`
	SenderID    int       `json:"sender_id"`
	ThreadID    string    `json:"thread_id,omitempty"`
	Subject     string    `json:"subject"`
	BodyMD      string    `json:"body_md"`
	Importance  string    `json:"importance"`
	AckRequired bool      `json:"ack_required"`
	CreatedTS   time.Time `json:"created_ts"`
	// Computed fields
	SenderName  string    `json:"sender_name,omitempty"`
	Recipients  []string  `json:"recipients,omitempty"`
}

// MessageRecipient represents a message recipient with read/ack status
type MessageRecipient struct {
	MessageID int        `json:"message_id"`
	AgentID   int        `json:"agent_id"`
	Kind      string     `json:"kind"` // to, cc, bcc
	ReadTS    *time.Time `json:"read_ts,omitempty"`
	AckTS     *time.Time `json:"ack_ts,omitempty"`
}

// FileReservation represents a file lock held by an agent
type FileReservation struct {
	ID          int        `json:"id"`
	ProjectID   int        `json:"project_id"`
	AgentID     int        `json:"agent_id"`
	PathPattern string     `json:"path_pattern"`
	Exclusive   bool       `json:"exclusive"`
	Reason      string     `json:"reason"`
	CreatedTS   time.Time  `json:"created_ts"`
	ExpiresTS   time.Time  `json:"expires_ts"`
	ReleasedTS  *time.Time `json:"released_ts,omitempty"`
	// Computed fields
	AgentName   string     `json:"agent_name,omitempty"`
	IsActive    bool       `json:"is_active"`
}

// AgentLink represents a contact approval between agents
type AgentLink struct {
	ID            int       `json:"id"`
	FromAgentID   int       `json:"from_agent_id"`
	ToAgentID     int       `json:"to_agent_id"`
	Status        string    `json:"status"`
	CreatedTS     time.Time `json:"created_ts"`
	ExpiresTS     time.Time `json:"expires_ts"`
}

// AgentStats holds statistics for an agent
type AgentStats struct {
	TotalMessages     int `json:"total_messages"`
	UnreadMessages    int `json:"unread_messages"`
	SentMessages      int `json:"sent_messages"`
	ActiveReservations int `json:"active_reservations"`
}

// Importance levels
const (
	ImportanceLow    = "low"
	ImportanceNormal = "normal"
	ImportanceHigh   = "high"
	ImportanceUrgent = "urgent"
)

// IsRead returns true if the message has been read by the recipient
func (mr *MessageRecipient) IsRead() bool {
	return mr.ReadTS != nil
}

// IsAcknowledged returns true if the message has been acknowledged
func (mr *MessageRecipient) IsAcknowledged() bool {
	return mr.AckTS != nil
}

// IsExpired returns true if the reservation has expired
func (fr *FileReservation) IsExpired() bool {
	return time.Now().After(fr.ExpiresTS)
}
