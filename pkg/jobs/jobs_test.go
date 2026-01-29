package jobs

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestJobStoreCreateDefaults(t *testing.T) {
	store := NewJobStore(1*time.Minute, 10)
	job := store.Create("scan")

	if job.ID == "" || !strings.HasPrefix(job.ID, "job-") {
		t.Fatalf("expected job id with prefix, got %q", job.ID)
	}
	if job.Type != "scan" {
		t.Fatalf("expected type scan, got %q", job.Type)
	}
	if job.Status != JobQueued {
		t.Fatalf("expected queued status, got %s", job.Status)
	}
	if job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set")
	}
}

func TestJobStoreStartFinishSuccess(t *testing.T) {
	store := NewJobStore(1*time.Minute, 10)
	job := store.Create("scan")
	err := store.Start(job.ID, func(context.Context) (any, error) {
		return map[string]string{"ok": "true"}, nil
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	final := waitForStatus(t, store, job.ID, JobSucceeded)
	if final.Result == nil {
		t.Fatalf("expected result to be set")
	}
	if final.Error != "" {
		t.Fatalf("expected no error, got %q", final.Error)
	}
	if final.FinishedAt == nil {
		t.Fatalf("expected finished_at to be set")
	}
}

func TestJobStoreCancelQueued(t *testing.T) {
	store := NewJobStore(1*time.Minute, 10)
	job := store.Create("scan")

	canceled, err := store.Cancel(job.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if canceled.Status != JobCanceled {
		t.Fatalf("expected canceled status, got %s", canceled.Status)
	}
	if canceled.Error != "job canceled" {
		t.Fatalf("expected cancel error, got %q", canceled.Error)
	}
	if canceled.FinishedAt == nil {
		t.Fatalf("expected finished_at to be set")
	}
}

func TestJobStoreExpireQueued(t *testing.T) {
	store := NewJobStore(5*time.Millisecond, 10)
	job := store.Create("scan")
	time.Sleep(10 * time.Millisecond)

	expired := waitForStatus(t, store, job.ID, JobExpired)
	if expired.Error != "job expired" {
		t.Fatalf("expected expire error, got %q", expired.Error)
	}
}

func TestJobStoreEvictsOldestTerminalWhenMaxExceeded(t *testing.T) {
	store := NewJobStore(1*time.Minute, 1)

	first := store.Create("scan")
	if err := store.Start(first.ID, func(context.Context) (any, error) { return "ok", nil }); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForStatus(t, store, first.ID, JobSucceeded)

	second := store.Create("scan")
	if _, ok := store.Get(first.ID); ok {
		t.Fatalf("expected oldest terminal job to be evicted when max exceeded")
	}
	if _, ok := store.Get(second.ID); !ok {
		t.Fatalf("expected newest job to remain")
	}
}

func waitForStatus(t *testing.T, store *JobStore, id string, status JobStatus) *Job {
	t.Helper()
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		job, ok := store.Get(id)
		if ok && job.Status == status {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for status %s", status)
	return nil
}
