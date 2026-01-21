package brief

import "fmt"

type Input struct {
	ID            string
	Title         string
	Summary       string
	Requirements  []string
	Acceptance    []string
	ResearchFiles []string
}

func Compose(in Input) string {
	return fmt.Sprintf(`PRD: %s
Title: %s

Summary:
%s

Requirements:
%v

Acceptance Criteria:
%v

Research:
%v
`, in.ID, in.Title, in.Summary, in.Requirements, in.Acceptance, in.ResearchFiles)
}
