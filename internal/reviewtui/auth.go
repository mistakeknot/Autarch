package reviewtui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/review"
)

type authTickMsg struct{}
type providerChoice struct {
	provider review.AuthProvider
	method   review.AuthMethod
}

func (m *Model) providerChoices() []providerChoice {
	var choices []providerChoice
	if m.auth == nil {
		return choices
	}
	for _, p := range m.auth.Providers {
		for _, method := range p.Methods {
			choices = append(choices, providerChoice{p, method})
		}
	}
	sort.SliceStable(choices, func(i, j int) bool {
		a, b := choices[i], choices[j]
		if (a.provider.ID == "openai-codex") != (b.provider.ID == "openai-codex") {
			return a.provider.ID == "openai-codex"
		}
		if a.provider.Name != b.provider.Name {
			return a.provider.Name < b.provider.Name
		}
		return a.method.Type == "oauth" && b.method.Type != "oauth"
	})
	return choices
}

func (m *Model) setAuth(state *review.AuthState) {
	key := ""
	if state != nil && state.Operation != nil && state.Operation.Prompt != nil {
		key = state.RuntimeID + "/" + state.Operation.ID + "/" + state.Operation.Prompt.ID
	}
	if key != m.authPromptKey {
		m.authInput.Reset()
		m.authPromptKey = key
		m.authSelection = 0
	}
	m.auth = state
	if key != "" {
		m.authInput.EchoMode = textinput.EchoPassword
		m.authInput.EchoCharacter = '•'
		m.authInput.Placeholder = "Enter response (masked)"
		m.authInput.Focus()
	}
}

func authTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return authTickMsg{} })
}

func (m *Model) pauseCapture() tea.Cmd {
	command := "pause"
	for _, s := range m.state.Sessions {
		if s.Project == m.project && s.Status == "paused" {
			command = "resume"
		}
	}
	return m.call(review.Request{Method: "capture.command", Text: command}, false)
}

func (m *Model) authKey(v tea.KeyMsg) tea.Cmd {
	key := v.String()
	if key == "esc" || key == "alt+p" {
		m.authOpen = false
		m.authInput.Reset()
		m.authInput.Blur()
		return nil
	}
	if m.authBusy {
		return nil
	}
	if m.auth == nil {
		if key == "enter" {
			m.authBusy = true
			return m.call(review.Request{Method: "auth.providers"}, false)
		}
		return nil
	}
	a := &review.AuthRequest{RuntimeID: m.auth.RuntimeID}
	if op := m.auth.Operation; op != nil && op.Status == "pending" {
		a.OperationID = op.ID
		if key == "ctrl+g" {
			m.authInput.Reset()
			m.authBusy = true
			return m.call(review.Request{Method: "auth.cancel", Auth: a}, false)
		}
		if op.ExpiresAt <= time.Now().UnixMilli() || op.Prompt == nil {
			return nil
		}
		prompt := op.Prompt
		if len(prompt.Options) > 0 {
			switch key {
			case "down":
				m.authSelection = min(m.authSelection+1, len(prompt.Options)-1)
			case "up":
				m.authSelection = max(0, m.authSelection-1)
			}
		}
		if key == "enter" || key == "ctrl+s" {
			a.PromptID, a.Value = prompt.ID, m.authInput.Value()
			if len(prompt.Options) > 0 {
				a.Value = prompt.Options[min(m.authSelection, len(prompt.Options)-1)].ID
			}
			// Empty text is meaningful for provider defaults and device-code fallback.
			m.authInput.Reset()
			m.authBusy = true
			return m.call(review.Request{Method: "auth.respond", Auth: a}, false)
		}
		if len(prompt.Options) > 0 {
			return nil
		}
		var cmd tea.Cmd
		m.authInput, cmd = m.authInput.Update(v)
		return cmd
	}
	choices := m.providerChoices()
	if key == "r" {
		m.authBusy = true
		return m.call(review.Request{Method: "auth.providers"}, false)
	}
	if key == "m" {
		m.authModels = !m.authModels
		m.authSelection = 0
		return nil
	}
	count := len(choices)
	if m.authModels {
		count = len(m.auth.Models)
	}
	switch key {
	case "up":
		m.authSelection = max(0, m.authSelection-1)
	case "down":
		m.authSelection = min(m.authSelection+1, max(0, count-1))
	case "enter", "ctrl+x":
		if count == 0 {
			return nil
		}
		if m.authModels {
			if key != "enter" {
				return nil
			}
			model := m.auth.Models[min(m.authSelection, count-1)]
			m.authOpen = false
			return m.call(review.Request{Method: "runtime.switch", Text: model.Provider + "/" + model.ID}, false)
		}
		choice := choices[min(m.authSelection, count-1)]
		a.Provider, a.AuthType = choice.provider.ID, choice.method.Type
		if key == "ctrl+x" {
			m.authBusy = true
			return m.call(review.Request{Method: "auth.logout", Auth: a}, false)
		}
		if !choice.method.Interactive {
			m.status = "Configure this provider's ambient credentials or endpoint in Pi, then press r to refresh"
			return nil
		}
		m.authBusy = true
		return m.call(review.Request{Method: "auth.login", Auth: a}, false)
	}
	return nil
}

func (m *Model) authView() string {
	var b strings.Builder
	b.WriteString("CONNECT PROVIDER · Pi / Flere\n\n")
	if m.auth == nil {
		b.WriteString("Loading provider registry… Enter retries · Esc returns\n")
		return b.String()
	}
	if op := m.auth.Operation; op != nil {
		fmt.Fprintf(&b, "%s · %s\n", op.Provider, op.Status)
		if op.ErrorCode != "" {
			message := "Authentication failed; reconnect to try again"
			if op.ErrorCode == "storage_error" {
				message = "Shared credential storage failed; check Flere storage permissions"
			}
			if op.ErrorCode == "catalog_error" {
				message = "Credentials changed; refresh the provider catalog"
			}
			b.WriteString(message + "\n")
		}
		if op.Status == "pending" {
			b.WriteString("Follow the sign-in instructions in Autarch's provider connection window.\nThat window is excluded from the pilot's capture picker.\n")
			if p := op.Prompt; p != nil {
				b.WriteString("\nSign-in response (see the provider connection window)\n")
				for i := range p.Options {
					mark := "  "
					if i == m.authSelection {
						mark = "› "
					}
					fmt.Fprintf(&b, "%sOption %d\n", mark, i+1)
				}
				if len(p.Options) == 0 {
					b.WriteString(m.authInput.View() + "\n")
				}
				b.WriteString("Enter submits\n")
			}
			b.WriteString("\nCtrl+G cancels · Esc returns; reopen with Alt+P\n")
			return b.String()
		}
	}
	if m.auth.Model != nil {
		fmt.Fprintf(&b, "Conversation: %s/%s\n", m.auth.Model.Provider, m.auth.Model.ID)
	}
	b.WriteString("Enter connects · Ctrl+X logs out · r refreshes · m models · Esc returns\n\n")
	start := max(0, m.authSelection-max(3, m.height-15)/2)
	if m.authModels {
		for i, model := range m.auth.Models {
			if i < start {
				continue
			}
			mark := "  "
			if i == m.authSelection {
				mark = "› "
			}
			fmt.Fprintf(&b, "%s%s/%s\n", mark, model.Provider, model.ID)
		}
	} else {
		for i, choice := range m.providerChoices() {
			if i < start {
				continue
			}
			mark := "  "
			if i == m.authSelection {
				mark = "› "
			}
			status := "disconnected"
			if choice.provider.Configured {
				status = "connected · " + choice.provider.Source
			}
			if !choice.method.Interactive {
				status += " · ambient/configured endpoint"
			}
			fmt.Fprintf(&b, "%s%s · %s · %s\n", mark, choice.provider.Name, choice.method.Name, status)
		}
	}
	return b.String()
}
