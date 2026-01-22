package agenttargets

type TargetType string

const (
	TargetDetected   TargetType = "detected"
	TargetPromptable TargetType = "promptable"
	TargetCommand    TargetType = "command"
)

type Target struct {
	Name    string
	Type    TargetType
	Command string
	Args    []string
	Env     map[string]string
}

type Registry struct {
	Targets map[string]Target
}
