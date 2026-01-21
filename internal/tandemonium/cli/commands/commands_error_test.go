package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandErrorWrapping(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		cmd     func() *cobra.Command
		args    []string
		wantErr string
	}{
		{name: "agent register", cmd: AgentCmd, args: []string{"register"}, wantErr: "agent register failed"},
		{name: "agent whois", cmd: AgentCmd, args: []string{"whois"}, wantErr: "agent whois failed"},
		{name: "agent health", cmd: AgentCmd, args: []string{"health"}, wantErr: "agent health failed"},
		{name: "lock reserve", cmd: LockCmd, args: []string{"reserve"}, wantErr: "lock reserve failed"},
		{name: "lock release", cmd: LockCmd, args: []string{"release"}, wantErr: "lock release failed"},
		{name: "lock renew", cmd: LockCmd, args: []string{"renew"}, wantErr: "lock renew failed"},
		{name: "lock force-release", cmd: LockCmd, args: []string{"force-release"}, wantErr: "lock force-release failed"},
		{name: "approve", cmd: ApproveCmd, args: nil, wantErr: "approve failed"},
		{name: "import", cmd: ImportCmd, args: nil, wantErr: "import failed"},
		{name: "plan", cmd: PlanCmd, args: nil, wantErr: "plan failed"},
		{name: "status", cmd: StatusCmd, args: []string{"--json"}, wantErr: "status failed"},
		{name: "doctor", cmd: DoctorCmd, args: []string{"--json"}, wantErr: "doctor failed"},
		{name: "cleanup", cmd: CleanupCmd, args: nil, wantErr: "cleanup failed"},
	}

	for _, tt := range tests {
		cmd := tt.cmd()
		cmd.SetOut(bytes.NewBuffer(nil))
		cmd.SetArgs(tt.args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("%s: expected error", tt.name)
		} else if !strings.Contains(err.Error(), tt.wantErr) {
			t.Fatalf("%s: expected %q, got %v", tt.name, tt.wantErr, err)
		}
	}
}
