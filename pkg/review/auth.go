package review

// Auth is a transient controller protocol. These types must never be added to
// State, receipts, turns, capture context, or evidence.
type AuthRequest struct {
	RuntimeID   string `json:"runtimeId,omitempty"`
	OperationID string `json:"operationId,omitempty"`
	PromptID    string `json:"promptId,omitempty"`
	Provider    string `json:"provider,omitempty"`
	AuthType    string `json:"authType,omitempty"`
	Value       string `json:"value,omitempty"`
}
type AuthMethod struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Interactive bool   `json:"interactive"`
}
type AuthProvider struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Configured bool         `json:"configured"`
	Source     string       `json:"source,omitempty"`
	Methods    []AuthMethod `json:"methods"`
}
type AuthModel struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
}
type AuthOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}
type AuthPrompt struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`
	Message     string       `json:"message"`
	Placeholder string       `json:"placeholder,omitempty"`
	Options     []AuthOption `json:"options,omitempty"`
}
type AuthEvent struct {
	Type            string `json:"type"`
	Message         string `json:"message,omitempty"`
	URL             string `json:"url,omitempty"`
	Instructions    string `json:"instructions,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
	VerificationURI string `json:"verificationUri,omitempty"`
	Links           []struct {
		URL   string `json:"url"`
		Label string `json:"label"`
	} `json:"links,omitempty"`
}
type AuthOperation struct {
	ID        string      `json:"id"`
	Provider  string      `json:"provider"`
	Status    string      `json:"status"`
	ExpiresAt int64       `json:"expiresAt"`
	Events    []AuthEvent `json:"events"`
	Prompt    *AuthPrompt `json:"prompt,omitempty"`
	ErrorCode string      `json:"errorCode,omitempty"`
}
type AuthState struct {
	DisplayID string         `json:"displayId,omitempty"`
	Project   string         `json:"project,omitempty"`
	RuntimeID string         `json:"runtimeId"`
	Providers []AuthProvider `json:"providers"`
	Models    []AuthModel    `json:"models"`
	Model     *AuthModel     `json:"model,omitempty"`
	Operation *AuthOperation `json:"operation,omitempty"`
}
