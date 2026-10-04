package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func bindingFiler(binding string) *CardFiler {
	return &CardFiler{Run: func(_ context.Context, _ []string, args ...string) BBResult {
		switch strings.Join(args[:min(len(args), 3)], " ") {
		case "tasks project list":
			return BBResult{Stdout: []byte(`{"projects":[{"id":"tp-1","name":"Shadow Workipedia","prefix":"SHWK"}]}`)}
		case "home binding tp-1":
			if binding == "" {
				return BBResult{Code: 1, Stderr: []byte("unknown command")}
			}
			return BBResult{Stdout: []byte(binding)}
		}
		return BBResult{Code: 1}
	}}
}

const confirmed = `{"home_project":"shadow-work","state":"confirmed"}`

// "shadow-workipedia" and then "SHWK" were accepted at file time and hidden in Home.
func TestResolveAskProjectRefusesAnythingButTheBoundProjectNamingIt(t *testing.T) {
	_, err := bindingFiler(confirmed).ResolveAskProject(context.Background(), "SHWK", "shadow-workipedia")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `expected project "shadow-work"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveAskProjectAcceptsTheBoundNameAndTheTasksKeyAlias(t *testing.T) {
	f := bindingFiler(confirmed)
	for _, in := range []string{"shadow-work", "SHWK", "shwk"} {
		got, err := f.ResolveAskProject(context.Background(), "SHWK", in)
		if err != nil || got != "shadow-work" {
			t.Fatalf("%s: got %q, %v", in, got, err)
		}
	}
}

func TestResolveAskProjectLeavesUnenforceableBindingsAlone(t *testing.T) {
	for name, b := range map[string]string{
		"suggested": `{"home_project":"shadow-work","state":"suggested"}`,
		"rejected":  `{"home_project":"shadow-work","state":"rejected"}`,
		"unbound":   `{"home_project":null,"state":null}`,
		"old Home":  "",
	} {
		got, err := bindingFiler(b).ResolveAskProject(context.Background(), "SHWK", "whatever")
		if err != nil || got != "whatever" {
			t.Fatalf("%s: got %q, %v", name, got, err)
		}
	}
}
