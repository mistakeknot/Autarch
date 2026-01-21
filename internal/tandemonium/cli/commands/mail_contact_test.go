package commands

import (
	"bytes"
	"os"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestMailPolicySetGet(t *testing.T) {
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

	set := MailCmd()
	set.SetOut(bytes.NewBuffer(nil))
	set.SetArgs([]string{"policy", "set", "--owner", "alice", "--policy", "contacts_only"})
	if err := set.Execute(); err != nil {
		t.Fatalf("set policy: %v", err)
	}

	get := MailCmd()
	out := bytes.NewBuffer(nil)
	get.SetOut(out)
	get.SetArgs([]string{"policy", "get", "--owner", "alice"})
	if err := get.Execute(); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("contacts_only")) {
		t.Fatalf("expected policy output")
	}
}

func TestMailContactRequestRespond(t *testing.T) {
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

	request := MailCmd()
	request.SetOut(bytes.NewBuffer(nil))
	request.SetArgs([]string{"contact", "request", "--requester", "alice", "--recipient", "bob"})
	if err := request.Execute(); err != nil {
		t.Fatalf("request: %v", err)
	}

	respond := MailCmd()
	respond.SetOut(bytes.NewBuffer(nil))
	respond.SetArgs([]string{"contact", "respond", "--requester", "alice", "--recipient", "bob", "--accept"})
	if err := respond.Execute(); err != nil {
		t.Fatalf("respond: %v", err)
	}
}
