package commands

import (
	"bytes"
	"os"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestMailReadMarksRead(t *testing.T) {
	dir := t.TempDir()
	if err := project.Init(dir); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	send := MailCmd()
	send.SetOut(bytes.NewBuffer(nil))
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Read", "--body", "Body", "--id", "msg-read"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	read := MailCmd()
	read.SetOut(bytes.NewBuffer(nil))
	read.SetArgs([]string{"read", "--id", "msg-read", "--recipient", "bob"})
	if err := read.Execute(); err != nil {
		t.Fatalf("read failed: %v", err)
	}
}
