package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestMailSendAndInbox(t *testing.T) {
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
	sendOut := bytes.NewBuffer(nil)
	send.SetOut(sendOut)
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Hello", "--body", "Body"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	inbox := MailCmd()
	inboxOut := bytes.NewBuffer(nil)
	inbox.SetOut(inboxOut)
	inbox.SetArgs([]string{"inbox", "--recipient", "bob"})
	if err := inbox.Execute(); err != nil {
		t.Fatalf("inbox failed: %v", err)
	}
	if !strings.Contains(inboxOut.String(), "Hello") {
		t.Fatalf("expected inbox output to contain subject")
	}
}

func TestMailSendCcBcc(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--cc", "carol", "--bcc", "dave", "--subject", "CcBcc", "--body", "Body"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	ccInbox := MailCmd()
	ccOut := bytes.NewBuffer(nil)
	ccInbox.SetOut(ccOut)
	ccInbox.SetArgs([]string{"inbox", "--recipient", "carol"})
	if err := ccInbox.Execute(); err != nil {
		t.Fatalf("cc inbox failed: %v", err)
	}
	if !strings.Contains(ccOut.String(), "CcBcc") {
		t.Fatalf("expected cc inbox output to contain subject")
	}

	bccInbox := MailCmd()
	bccOut := bytes.NewBuffer(nil)
	bccInbox.SetOut(bccOut)
	bccInbox.SetArgs([]string{"inbox", "--recipient", "dave"})
	if err := bccInbox.Execute(); err != nil {
		t.Fatalf("bcc inbox failed: %v", err)
	}
	if !strings.Contains(bccOut.String(), "CcBcc") {
		t.Fatalf("expected bcc inbox output to contain subject")
	}
}

func TestMailInboxUrgentOnly(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--subject", "Normal", "--body", "Body", "--importance", "normal"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	sendUrgent := MailCmd()
	sendUrgent.SetOut(bytes.NewBuffer(nil))
	sendUrgent.SetArgs([]string{"send", "--to", "bob", "--subject", "Urgent", "--body", "Body", "--importance", "urgent"})
	if err := sendUrgent.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	inbox := MailCmd()
	out := bytes.NewBuffer(nil)
	inbox.SetOut(out)
	inbox.SetArgs([]string{"inbox", "--recipient", "bob", "--urgent-only"})
	if err := inbox.Execute(); err != nil {
		t.Fatalf("inbox failed: %v", err)
	}
	if strings.Contains(out.String(), "Normal") || !strings.Contains(out.String(), "Urgent") {
		t.Fatalf("expected only urgent message")
	}
}

func TestMailInboxSinceFilters(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--subject", "Soon", "--body", "Body"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	inbox := MailCmd()
	out := bytes.NewBuffer(nil)
	inbox.SetOut(out)
	inbox.SetArgs([]string{"inbox", "--recipient", "bob", "--since", "2999-01-01T00:00:00Z"})
	if err := inbox.Execute(); err != nil {
		t.Fatalf("inbox failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected no messages for future since filter")
	}
}

func TestMailInboxJSONOutput(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--subject", "Json", "--body", "Body"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	inbox := MailCmd()
	out := bytes.NewBuffer(nil)
	inbox.SetOut(out)
	inbox.SetArgs([]string{"inbox", "--recipient", "bob", "--json"})
	if err := inbox.Execute(); err != nil {
		t.Fatalf("inbox failed: %v", err)
	}
	var payload struct {
		Messages []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Subject != "Json" {
		t.Fatalf("expected json message output")
	}
}

func TestMailSendWithAttachments(t *testing.T) {
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
	attachPath := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(attachPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	send.SetArgs([]string{"send", "--to", "bob", "--subject", "Attach", "--body", "Body", "--id", "msg-attach-cli", "--attach", attachPath + "::spec"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	db, closeDB, err := openStateDB()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer closeDB()
	attachments, err := storage.ListAttachments(db, "msg-attach-cli")
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment")
	}
	stored := filepath.Join(project.AttachmentsDir(dir), attachments[0].BlobHash[:2], attachments[0].BlobHash)
	if _, err := os.Stat(stored); err != nil {
		t.Fatalf("expected stored attachment: %v", err)
	}
}

func TestMailCommandUsage(t *testing.T) {
	if MailCmd().Use != "mail" {
		t.Fatalf("unexpected Use")
	}
}

func TestMailSendRequiresRecipient(t *testing.T) {
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

	cmd := MailCmd()
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"send", "--subject", "Hello", "--body", "Body"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for missing recipient")
	}
}

func TestMailSendWritesStateDB(t *testing.T) {
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

	cmd := MailCmd()
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"send", "--to", "bob", "--subject", "Hello", "--body", "Body"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tandemonium", "state.db")); err != nil {
		t.Fatalf("expected state.db: %v", err)
	}
}

func TestMailContactPolicy(t *testing.T) {
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

	policy := MailCmd()
	policy.SetOut(bytes.NewBuffer(nil))
	policy.SetArgs([]string{"policy", "set", "--owner", "bob", "--policy", "contacts_only"})
	if err := policy.Execute(); err != nil {
		t.Fatalf("policy set failed: %v", err)
	}

	send := MailCmd()
	send.SetOut(bytes.NewBuffer(nil))
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Hello", "--body", "Body"})
	if err := send.Execute(); err == nil {
		t.Fatalf("expected send to fail without contact")
	}

	request := MailCmd()
	request.SetOut(bytes.NewBuffer(nil))
	request.SetArgs([]string{"contact", "request", "--requester", "alice", "--recipient", "bob"})
	if err := request.Execute(); err != nil {
		t.Fatalf("contact request failed: %v", err)
	}

	respond := MailCmd()
	respond.SetOut(bytes.NewBuffer(nil))
	respond.SetArgs([]string{"contact", "respond", "--requester", "alice", "--recipient", "bob", "--accept"})
	if err := respond.Execute(); err != nil {
		t.Fatalf("contact respond failed: %v", err)
	}

	send = MailCmd()
	send.SetOut(bytes.NewBuffer(nil))
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Hello", "--body", "Body"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed after contact: %v", err)
	}
}

func TestMailInboxJsonIncludesRecipientMetadata(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--cc", "carol", "--bcc", "dave", "--subject", "CcBcc", "--body", "Body"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	inbox := MailCmd()
	out := bytes.NewBuffer(nil)
	inbox.SetOut(out)
	inbox.SetArgs([]string{"inbox", "--recipient", "bob", "--json"})
	if err := inbox.Execute(); err != nil {
		t.Fatalf("inbox failed: %v", err)
	}

	var payload struct {
		Messages []struct {
			To  []string `json:"to"`
			Cc  []string `json:"cc"`
			Bcc []string `json:"bcc"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(payload.Messages) != 1 {
		t.Fatalf("expected 1 message")
	}
	if len(payload.Messages[0].Cc) != 1 || len(payload.Messages[0].Bcc) != 1 {
		t.Fatalf("expected cc/bcc metadata")
	}
}

func TestMailSearchPaginationJson(t *testing.T) {
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

	for i := 0; i < 3; i++ {
		send := MailCmd()
		send.SetOut(bytes.NewBuffer(nil))
		send.SetArgs([]string{"send", "--to", "bob", "--subject", "Hello", "--body", "Body"})
		if err := send.Execute(); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}

	search := MailCmd()
	out := bytes.NewBuffer(nil)
	search.SetOut(out)
	search.SetArgs([]string{"search", "--query", "Hello", "--limit", "2", "--json"})
	if err := search.Execute(); err != nil {
		t.Fatalf("search failed: %v", err)
	}

	var payload struct {
		Messages  []storage.Message `json:"messages"`
		NextToken string            `json:"next_token"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload.NextToken == "" {
		t.Fatalf("expected next token")
	}
}

func TestMailSummarizeThreadLLM(t *testing.T) {
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

	cmdPath := filepath.Join(dir, "summary.sh")
	script := "#!/bin/sh\necho '{\"summary\":{\"participants\":[\"alice\"],\"key_points\":[\"p1\"],\"action_items\":[]},\"examples\":[{\"id\":\"m1\",\"subject\":\"Hello\",\"body\":\"Body\"}]}'\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, ".tandemonium", "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[llm_summary]\ncommand = \""+cmdPath+"\"\ntimeout_seconds = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	send := MailCmd()
	send.SetOut(bytes.NewBuffer(nil))
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Thread", "--body", "Body", "--id", "msg-thread", "--thread", "thread-llm"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	summarize := MailCmd()
	out := bytes.NewBuffer(nil)
	summarize.SetOut(out)
	summarize.SetArgs([]string{"summarize", "--thread", "thread-llm", "--llm", "--examples", "--json"})
	if err := summarize.Execute(); err != nil {
		t.Fatalf("summarize failed: %v", err)
	}

	var payload struct {
		KeyPoints []string `json:"key_points"`
		Examples  []struct {
			ID string `json:"id"`
		} `json:"examples"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(payload.KeyPoints) == 0 || len(payload.Examples) == 0 {
		t.Fatalf("expected llm summary output")
	}
}

func TestMailSummarizeDryRunUsesLLM(t *testing.T) {
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

	cmdPath := filepath.Join(dir, "summary.sh")
	script := "#!/bin/sh\necho '{\"summary\":{\"participants\":[\"alice\"],\"key_points\":[\"p1\"],\"action_items\":[]},\"examples\":[{\"id\":\"m1\",\"subject\":\"Hello\",\"body\":\"Body\"}]}'\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, ".tandemonium", "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[llm_summary]\ncommand = \""+cmdPath+"\"\ntimeout_seconds = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summarize := MailCmd()
	out := bytes.NewBuffer(nil)
	summarize.SetOut(out)
	summarize.SetArgs([]string{"summarize", "--dry-run", "--json"})
	if err := summarize.Execute(); err != nil {
		t.Fatalf("summarize failed: %v", err)
	}

	var payload struct {
		KeyPoints []string `json:"key_points"`
		Examples  []struct {
			ID string `json:"id"`
		} `json:"examples"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(payload.KeyPoints) == 0 || len(payload.Examples) == 0 {
		t.Fatalf("expected llm summary output")
	}
}

func TestMailReplyDefaults(t *testing.T) {
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
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Hello", "--body", "Body", "--id", "msg-orig", "--thread", "thread-1", "--importance", "urgent", "--ack"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	reply := MailCmd()
	reply.SetOut(bytes.NewBuffer(nil))
	reply.SetArgs([]string{"reply", "--id", "msg-orig", "--from", "bob", "--body", "Reply"})
	if err := reply.Execute(); err != nil {
		t.Fatalf("reply failed: %v", err)
	}

	inbox := MailCmd()
	out := bytes.NewBuffer(nil)
	inbox.SetOut(out)
	inbox.SetArgs([]string{"inbox", "--recipient", "alice", "--json"})
	if err := inbox.Execute(); err != nil {
		t.Fatalf("inbox failed: %v", err)
	}

	var payload struct {
		Messages []struct {
			ID          string   `json:"id"`
			ThreadID    string   `json:"thread_id"`
			Subject     string   `json:"subject"`
			Importance  string   `json:"importance"`
			AckRequired bool     `json:"ack_required"`
			Recipient   string   `json:"recipient"`
			To          []string `json:"to"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(payload.Messages) != 1 {
		t.Fatalf("expected 1 reply message")
	}
	replyMsg := payload.Messages[0]
	if replyMsg.Subject != "Re: Hello" {
		t.Fatalf("expected reply subject, got %q", replyMsg.Subject)
	}
	if replyMsg.ThreadID != "thread-1" {
		t.Fatalf("expected thread id thread-1, got %q", replyMsg.ThreadID)
	}
	if replyMsg.Importance != "urgent" {
		t.Fatalf("expected urgent importance, got %q", replyMsg.Importance)
	}
	if !replyMsg.AckRequired {
		t.Fatalf("expected ack required")
	}
	if replyMsg.Recipient != "alice" {
		t.Fatalf("expected recipient alice")
	}
	if len(replyMsg.To) != 1 || replyMsg.To[0] != "alice" {
		t.Fatalf("expected to metadata for alice")
	}
}

func TestMailSummarizeDryRunInvalidOutput(t *testing.T) {
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

	cmdPath := filepath.Join(dir, "summary.sh")
	script := "#!/bin/sh\necho 'not-json'\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, ".tandemonium", "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[llm_summary]\ncommand = \""+cmdPath+"\"\ntimeout_seconds = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summarize := MailCmd()
	summarize.SetOut(bytes.NewBuffer(nil))
	summarize.SetArgs([]string{"summarize", "--dry-run"})
	if err := summarize.Execute(); err == nil {
		t.Fatalf("expected summarize to fail")
	} else if !strings.Contains(err.Error(), "llm command output invalid") {
		t.Fatalf("expected invalid output error, got %v", err)
	}
}

func TestMailSummarizeLLMInvalidOutput(t *testing.T) {
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

	cmdPath := filepath.Join(dir, "summary.sh")
	script := "#!/bin/sh\necho 'not-json'\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, ".tandemonium", "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[llm_summary]\ncommand = \""+cmdPath+"\"\ntimeout_seconds = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	send := MailCmd()
	send.SetOut(bytes.NewBuffer(nil))
	send.SetArgs([]string{"send", "--to", "bob", "--from", "alice", "--subject", "Thread", "--body", "Body", "--id", "msg-thread-invalid", "--thread", "thread-invalid"})
	if err := send.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	summarize := MailCmd()
	summarize.SetOut(bytes.NewBuffer(nil))
	summarize.SetArgs([]string{"summarize", "--thread", "thread-invalid", "--llm"})
	if err := summarize.Execute(); err == nil {
		t.Fatalf("expected summarize to fail")
	} else {
		msg := err.Error()
		if !strings.Contains(msg, "summarize failed") || !strings.Contains(msg, "llm command output invalid") {
			t.Fatalf("expected wrapped invalid output error, got %v", err)
		}
	}
}

func TestMailCommandErrorWrapping(t *testing.T) {
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

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "send", args: []string{"send", "--subject", "Hello", "--body", "Body"}, wantErr: "mail send failed"},
		{name: "inbox", args: []string{"inbox"}, wantErr: "mail inbox failed"},
		{name: "ack", args: []string{"ack", "--recipient", "bob"}, wantErr: "mail ack failed"},
		{name: "read", args: []string{"read", "--recipient", "bob"}, wantErr: "mail read failed"},
		{name: "search", args: []string{"search"}, wantErr: "mail search failed"},
		{name: "reply", args: []string{"reply"}, wantErr: "mail reply failed"},
		{name: "summarize", args: []string{"summarize"}, wantErr: "mail summarize failed"},
		{name: "policy set", args: []string{"policy", "set"}, wantErr: "mail policy set failed"},
		{name: "policy get", args: []string{"policy", "get"}, wantErr: "mail policy get failed"},
		{name: "contact request", args: []string{"contact", "request"}, wantErr: "mail contact request failed"},
		{name: "contact respond", args: []string{"contact", "respond"}, wantErr: "mail contact respond failed"},
		{name: "contact list", args: []string{"contact", "list"}, wantErr: "mail contact list failed"},
	}

	for _, tt := range tests {
		cmd := MailCmd()
		cmd.SetOut(bytes.NewBuffer(nil))
		cmd.SetArgs(tt.args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("%s: expected error", tt.name)
		} else if !strings.Contains(err.Error(), tt.wantErr) {
			t.Fatalf("%s: expected %q, got %v", tt.name, tt.wantErr, err)
		}
	}
}

func TestMailInboxPlainTextIncludesNextToken(t *testing.T) {
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

	for i := 0; i < 3; i++ {
		send := MailCmd()
		send.SetOut(bytes.NewBuffer(nil))
		send.SetArgs([]string{"send", "--to", "bob", "--subject", "Hello", "--body", "Body"})
		if err := send.Execute(); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}

	inbox := MailCmd()
	out := bytes.NewBuffer(nil)
	inbox.SetOut(out)
	inbox.SetArgs([]string{"inbox", "--recipient", "bob", "--limit", "2"})
	if err := inbox.Execute(); err != nil {
		t.Fatalf("inbox failed: %v", err)
	}
	if !strings.Contains(out.String(), "next_token ") {
		t.Fatalf("expected next_token in output")
	}
}
