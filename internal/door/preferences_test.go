package door

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
	"github.com/mistakeknot/autarch/pkg/review"
)

func TestWorkbenchPreferencesPreserveUnknownAndRestoreWithoutDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte(`{"future":{"keep":true},"projects":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	p := WorkbenchPreference{Outcome: "Outcome", Task: review.TaskIdentity{TrackerUUID: "uuid", BeadID: "bead", Assignee: "seat", Qualification: "qualified"}, Target: workbenchTarget(), AccountLabel: "work", Draft: "unsent\ndraft", RecoveryHandoffID: "uncertain-1", RecoveryTarget: workbenchTarget()}
	if err := SaveWorkbenchPreference(path, "/project", p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadWorkbenchPreference(path, "/project")
	if err != nil || got.Draft != p.Draft || got.Target.Key() != p.Target.Key() || got.AccountLabel != "work" || got.Outcome != "Outcome" || got.RecoveryHandoffID != p.RecoveryHandoffID || got.RecoveryTarget != p.RecoveryTarget {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	b, _ := os.ReadFile(path)
	var root map[string]json.RawMessage
	if json.Unmarshal(b, &root) != nil || root["future"] == nil {
		t.Fatalf("unknown key lost: %s", b)
	}
	m := productModelFixture(t)
	client := &workbenchClient{}
	m.workbench.client = client
	m.workbench.preferencePath = path
	m.workbench.preference = got
	m.restoreWorkbenchPreference(got)
	if len(client.calls) != 0 || m.workbench.draft != p.Draft || !m.workbench.openRequired || m.workbench.recoveryHandoffID != p.RecoveryHandoffID {
		t.Fatal("restore delivered input or lost draft")
	}
}

func TestStaleExactTargetNeverRebindsByDisplayName(t *testing.T) {
	m := workbenchModelFixture(t)
	stale := workbenchTarget()
	stale.PanePID++
	m.restoreWorkbenchPreference(WorkbenchPreference{Target: stale, Draft: "keep me"})
	m.applyWorkbenchPanes([]agenttransport.Pane{{Target: workbenchTarget(), SessionName: "autarch", WindowName: "agent", Command: "codex"}})
	if !m.workbench.targetStale || m.workbench.target.PanePID != stale.PanePID || m.workbench.draft != "keep me" {
		t.Fatalf("stale target rebound: %+v", m.workbench)
	}
}

func TestReopenRestoresOutcomeTaskExactPaneAndOnlyUnsentDraft(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".autarch", "preferences.json")
	t.Setenv("AUTARCH_PREFERENCES_FILE", path)
	target := workbenchTarget()
	want := WorkbenchPreference{
		Outcome: "Make directing existing agents reliable",
		Task:    review.TaskIdentity{Tracker: "workspace", TrackerUUID: "tracker-uuid", BeadID: "Sylveste-fuwn", Assignee: "codex", Qualification: "qualified"},
		Target:  target, AccountLabel: "daily Codex", Draft: "unsent correction",
	}
	if err := SaveWorkbenchPreference(path, root, want); err != nil {
		t.Fatal(err)
	}
	reopened := NewProductModel(root)
	if reopened.workbench.outcome != want.Outcome || reopened.workbench.task != want.Task || reopened.workbench.target != target || reopened.workbench.accountLabel != want.AccountLabel || reopened.workbench.draft != want.Draft || !reopened.workbench.targetStale {
		t.Fatalf("reopen lost selected workbench state or trusted a stale target: %+v", reopened.workbench)
	}
	reopened.applyWorkbenchPanes([]agenttransport.Pane{{Target: target, Command: "codex"}})
	if reopened.workbench.targetStale {
		t.Fatal("fresh exact target did not revalidate")
	}

	reopened.workbench.draft = ""
	reopened.saveWorkbenchPreference()
	afterSend, err := LoadWorkbenchPreference(path, root)
	if err != nil || afterSend.Draft != "" {
		t.Fatalf("sent text reopened as an unsent draft: %+v err=%v", afterSend, err)
	}
}
