package storage

import (
	"testing"
	"time"
)

func TestSendAndFetchInbox(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	msg := Message{
		ID:          "msg-1",
		ThreadID:    "thread-1",
		Sender:      "alice",
		Subject:     "Hello",
		Body:        "Body",
		CreatedAt:   "2026-01-15T00:00:00Z",
		Importance:  "high",
		AckRequired: true,
	}
	if err := SendMessage(db, msg, []string{"bob", "carol"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	inbox, err := FetchInbox(db, "bob", 10)
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 message, got %d", len(inbox))
	}
	got := inbox[0]
	if got.Message.ID != msg.ID {
		t.Fatalf("expected message id %s, got %s", msg.ID, got.Message.ID)
	}
	if got.Recipient != "bob" {
		t.Fatalf("expected recipient bob, got %s", got.Recipient)
	}
	if got.Message.AckRequired != true {
		t.Fatalf("expected ack required true")
	}
}

func TestAckMessageMarksAckTs(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	msg := Message{
		ID:         "msg-2",
		ThreadID:   "thread-2",
		Sender:     "alice",
		Subject:    "Hello",
		Body:       "Body",
		CreatedAt:  "2026-01-15T00:00:00Z",
		Importance: "normal",
	}
	if err := SendMessage(db, msg, []string{"bob"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	ackTs := "2026-01-15T01:00:00Z"
	if err := AckMessage(db, "msg-2", "bob", ackTs); err != nil {
		t.Fatalf("ack message: %v", err)
	}

	inbox, err := FetchInbox(db, "bob", 10)
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if inbox[0].AckAt != ackTs {
		t.Fatalf("expected ack %s, got %s", ackTs, inbox[0].AckAt)
	}
}

func TestReservePathsConflicts(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	res, err := ReservePaths(db, "alice", []string{"a.go"}, true, "work", time.Hour)
	if err != nil {
		t.Fatalf("reserve paths: %v", err)
	}
	if len(res.Granted) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(res.Granted))
	}

	res, err = ReservePaths(db, "bob", []string{"a.go"}, true, "work", time.Hour)
	if err != nil {
		t.Fatalf("reserve paths: %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(res.Conflicts))
	}

	if _, err := ReleasePaths(db, "alice", []string{"a.go"}); err != nil {
		t.Fatalf("release paths: %v", err)
	}

	res, err = ReservePaths(db, "bob", []string{"a.go"}, true, "work", time.Hour)
	if err != nil {
		t.Fatalf("reserve paths: %v", err)
	}
	if len(res.Granted) != 1 {
		t.Fatalf("expected 1 grant after release, got %d", len(res.Granted))
	}
}

func TestSearchMessagesFindsSubject(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	msg := Message{
		ID:         "msg-3",
		ThreadID:   "thread-3",
		Sender:     "alice",
		Subject:    "Hello World",
		Body:       "Body",
		CreatedAt:  "2026-01-15T00:00:00Z",
		Importance: "normal",
	}
	if err := SendMessage(db, msg, []string{"bob"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	results, err := SearchMessages(db, "World", 10)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "msg-3" {
		t.Fatalf("expected msg-3, got %s", results[0].ID)
	}
}

func TestSummarizeThreadCollectsParticipants(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	msg := Message{
		ID:         "msg-4",
		ThreadID:   "thread-4",
		Sender:     "alice",
		Subject:    "Hello",
		Body:       "Body",
		CreatedAt:  "2026-01-15T00:00:00Z",
		Importance: "normal",
	}
	if err := SendMessage(db, msg, []string{"bob", "carol"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	summary, err := SummarizeThread(db, "thread-4")
	if err != nil {
		t.Fatalf("summarize thread: %v", err)
	}
	if summary.MessageCount != 1 {
		t.Fatalf("expected 1 message, got %d", summary.MessageCount)
	}

	want := map[string]bool{"alice": true, "bob": true, "carol": true}
	if len(summary.Participants) != len(want) {
		t.Fatalf("expected %d participants, got %d", len(want), len(summary.Participants))
	}
	for _, p := range summary.Participants {
		if !want[p] {
			t.Fatalf("unexpected participant %s", p)
		}
	}
}

func TestFetchInboxFilters(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	base := "2026-01-16T00:00:00Z"
	late := "2026-01-16T02:00:00Z"
	if err := SendMessage(db, Message{
		ID:         "msg-old",
		ThreadID:   "thread-filters",
		Sender:     "alice",
		Subject:    "Old",
		Body:       "Body",
		CreatedAt:  base,
		Importance: "normal",
	}, []string{"bob"}); err != nil {
		t.Fatalf("send message: %v", err)
	}
	if err := SendMessage(db, Message{
		ID:         "msg-urgent",
		ThreadID:   "thread-filters",
		Sender:     "alice",
		Subject:    "Urgent",
		Body:       "Body",
		CreatedAt:  late,
		Importance: "urgent",
	}, []string{"bob"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	urgent, err := FetchInboxWithFilters(db, "bob", 10, "", true)
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(urgent) != 1 || urgent[0].Message.ID != "msg-urgent" {
		t.Fatalf("expected urgent message")
	}

	since, err := FetchInboxWithFilters(db, "bob", 10, "2026-01-16T01:00:00Z", false)
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(since) != 1 || since[0].Message.ID != "msg-urgent" {
		t.Fatalf("expected since filter message")
	}
}

func TestMarkMessageReadSetsReadAt(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	if err := SendMessage(db, Message{
		ID:         "msg-read",
		ThreadID:   "thread-read",
		Sender:     "alice",
		Subject:    "Read",
		Body:       "Body",
		CreatedAt:  "2026-01-16T00:00:00Z",
		Importance: "normal",
	}, []string{"bob"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	readTs := "2026-01-16T01:00:00Z"
	if err := MarkMessageRead(db, "msg-read", "bob", readTs); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	inbox, err := FetchInbox(db, "bob", 10)
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].ReadAt != readTs {
		t.Fatalf("expected read timestamp")
	}
}

func TestListActiveReservations(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	if _, err := ReservePaths(db, "alice", []string{"a.go"}, true, "work", time.Hour); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	reservations, err := ListActiveReservations(db, 10)
	if err != nil {
		t.Fatalf("list reservations: %v", err)
	}
	if len(reservations) != 1 {
		t.Fatalf("expected 1 reservation, got %d", len(reservations))
	}
}

func TestFetchInboxPagination(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	msgs := []Message{
		{ID: "m1", Sender: "a", Subject: "1", Body: "b", CreatedAt: "2026-01-01T00:00:01Z"},
		{ID: "m2", Sender: "a", Subject: "2", Body: "b", CreatedAt: "2026-01-01T00:00:02Z"},
		{ID: "m3", Sender: "a", Subject: "3", Body: "b", CreatedAt: "2026-01-01T00:00:03Z"},
	}
	for _, msg := range msgs {
		if err := SendMessage(db, msg, []string{"bob"}); err != nil {
			t.Fatalf("send message: %v", err)
		}
	}

	page1, next, err := FetchInboxPage(db, "bob", 2, "", false, "")
	if err != nil {
		t.Fatalf("fetch page1: %v", err)
	}
	if len(page1) != 2 || next == "" {
		t.Fatalf("expected page1 with next token")
	}

	page2, next2, err := FetchInboxPage(db, "bob", 2, "", false, next)
	if err != nil {
		t.Fatalf("fetch page2: %v", err)
	}
	if len(page2) != 1 || next2 != "" {
		t.Fatalf("expected page2 final")
	}
}

func TestSearchMessagesPagination(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	msgs := []Message{
		{ID: "s1", Sender: "alice", Subject: "Hello", Body: "Body", CreatedAt: "2026-01-01T00:00:01Z"},
		{ID: "s2", Sender: "alice", Subject: "Hello", Body: "Body", CreatedAt: "2026-01-01T00:00:02Z"},
		{ID: "s3", Sender: "alice", Subject: "Hello", Body: "Body", CreatedAt: "2026-01-01T00:00:03Z"},
	}
	for _, msg := range msgs {
		if err := SendMessage(db, msg, []string{"bob"}); err != nil {
			t.Fatalf("send message: %v", err)
		}
	}

	page1, next, err := SearchMessagesPage(db, "Hello", 2, "")
	if err != nil {
		t.Fatalf("search page1: %v", err)
	}
	if len(page1) != 2 || next == "" {
		t.Fatalf("expected page1 with next token")
	}

	page2, next2, err := SearchMessagesPage(db, "Hello", 2, next)
	if err != nil {
		t.Fatalf("search page2: %v", err)
	}
	if len(page2) != 1 || next2 != "" {
		t.Fatalf("expected page2 final")
	}
}
