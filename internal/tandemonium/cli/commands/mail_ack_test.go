package commands

import (
	"bytes"
	"os"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestMailAckMarksAcked(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Hello", "--body", "Body", "--id", "msg-ack"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	ack := MailCmd()
	ack.SetOut(bytes.NewBuffer(nil))
	ack.SetArgs([]string{"ack", "--id", "msg-ack", "--recipient", "bob"})
	if err := ack.Execute(); err != nil {
		t.Fatalf("ack failed: %v", err)
	}
}

func TestMailSearchFindsMessage(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Searchable", "--body", "Body", "--id", "msg-search"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	search := MailCmd()
	out := bytes.NewBuffer(nil)
	search.SetOut(out)
	search.SetArgs([]string{"search", "--query", "Searchable"})
	if err := search.Execute(); err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("msg-search")) {
		t.Fatalf("expected search output")
	}
}

func TestMailSummarizeThread(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Thread", "--body", "Body", "--id", "msg-thread", "--thread", "thread-1"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	summarize := MailCmd()
	out := bytes.NewBuffer(nil)
	summarize.SetOut(out)
	summarize.SetArgs([]string{"summarize", "--thread", "thread-1"})
	if err := summarize.Execute(); err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("alice")) {
		t.Fatalf("expected summarize output")
	}
}
