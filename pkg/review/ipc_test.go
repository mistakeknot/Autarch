package review

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestControllerReconnectPreservesSameSession(t *testing.T) {
	dir, _ := os.MkdirTemp("/tmp", "review-")
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "ipc.sock")
	store, _ := Open(dir)
	server, err := Listen(socket, store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve()
	client := Client{Socket: socket}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	project := t.TempDir()
	session := Session{ID: "recording", Status: "recording", WindowID: 42}
	if _, err = client.Call(ctx, Request{Method: "session.save", Project: project, Session: &session}); err != nil {
		t.Fatal(err)
	}
	// A brand new client represents a closed and reopened TUI.
	client = Client{Socket: socket}
	response, err := client.Call(ctx, Request{Method: "state"})
	if err != nil {
		t.Fatal(err)
	}
	if response.State.Sessions["recording"].Status != "recording" {
		t.Fatal("TUI reconnect interrupted recording")
	}
}

func TestSecondControllerCannotStealSocket(t *testing.T) {
	dir, _ := os.MkdirTemp("/tmp", "review-")
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "ipc.sock")
	store, _ := Open(dir)
	first, err := Listen(socket, store)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := Listen(socket, store); err == nil {
		second.Close()
		t.Fatal("second writer stole controller socket")
	}
}
