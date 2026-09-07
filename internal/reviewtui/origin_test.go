package reviewtui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestTerminalOriginUsesOnlyClientsShowingInvokingPane(t *testing.T) {
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name + " " + args[0] {
		case "tmux display-message":
			if !strings.Contains(strings.Join(args, " "), "%7") {
				t.Fatal("missing exact target pane")
			}
			return []byte("$2\t@3\t%7\t/work tree\n"), nil
		case "tmux list-clients":
			return []byte("100\t%7\n200\t%8\n"), nil
		case "ps -axo":
			return []byte("100 50\n50 1\n200 60\n60 1\n300 70\n70 1\n"), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	}
	origin := resolveTerminalOrigin(context.Background(), 300, "%7", "/cwd", run)
	if origin.Pane != "%7" || origin.Window != "@3" || origin.Session != "$2" || origin.Cwd != "/work tree" {
		t.Fatalf("lost pane metadata: %+v", origin)
	}
	if !reflect.DeepEqual(origin.ProcessIDs, []int{100, 50}) {
		t.Fatalf("included another pane or tmux server ancestry: %v", origin.ProcessIDs)
	}
}

func TestDetachedTmuxDoesNotFallBackToServerTerminal(t *testing.T) {
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "ps" {
			return []byte("300 70\n70 1\n"), nil
		}
		if args[0] == "display-message" {
			return []byte("$2\t@3\t%7\t/work\n"), nil
		}
		return nil, nil
	}
	if got := resolveTerminalOrigin(context.Background(), 300, "%7", "/cwd", run); len(got.ProcessIDs) != 0 {
		t.Fatalf("detached source falsely attached: %+v", got)
	}
	if got := resolveTerminalOrigin(context.Background(), 300, "", "/cwd", run); !reflect.DeepEqual(got.ProcessIDs, []int{300, 70}) {
		t.Fatalf("plain terminal ancestry missing: %+v", got)
	}
}
