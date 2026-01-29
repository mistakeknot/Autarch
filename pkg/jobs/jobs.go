package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
	JobExpired   JobStatus = "expired"
	JobStalled   JobStatus = "stalled"
	JobRetrying  JobStatus = "retrying"
	JobPaused    JobStatus = "paused"
)

type Job struct {
	ID         string
	Type       string
	Status     JobStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	Error      string
	Result     any
	cancel     context.CancelFunc
}

type JobStore struct {
	mu   sync.Mutex
	jobs map[string]*Job
	ttl  time.Duration
	max  int
}

func NewJobStore(ttl time.Duration, max int) *JobStore {
	return &JobStore{jobs: make(map[string]*Job), ttl: ttl, max: max}
}

func (s *JobStore) Create(jobType string) *Job {
	job := &Job{
		ID:        newJobID(),
		Type:      jobType,
		Status:    JobQueued,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.pruneLocked(time.Now())
	s.mu.Unlock()
	return cloneJob(job)
}

func (s *JobStore) Start(id string, fn func(ctx context.Context) (any, error)) error {
	s.mu.Lock()
	job, ok := s.jobs[id]
	if !ok {
		s.mu.Unlock()
		return errors.New("job not found")
	}
	if job.Status != JobQueued {
		s.mu.Unlock()
		return errors.New("job not queued")
	}
	ctx, cancel := context.WithCancel(context.Background())
	job.cancel = cancel
	now := time.Now()
	job.Status = JobRunning
	job.StartedAt = &now
	job.UpdatedAt = now
	s.mu.Unlock()

	go func() {
		result, err := fn(ctx)
		s.finish(id, result, err)
	}()

	return nil
}

func (s *JobStore) Cancel(id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, errors.New("job not found")
	}
	if isTerminal(job.Status) {
		return cloneJob(job), errors.New("job already complete")
	}
	switch job.Status {
	case JobQueued, JobRunning, JobPaused, JobStalled, JobRetrying:
		now := time.Now()
		job.Status = JobCanceled
		job.UpdatedAt = now
		job.FinishedAt = &now
		job.Error = "job canceled"
		if job.cancel != nil {
			job.cancel()
		}
		return cloneJob(job), nil
	default:
		return cloneJob(job), errors.New("job not cancelable")
	}
}

func (s *JobStore) Get(id string) (*Job, bool) {
	s.mu.Lock()
	s.pruneLocked(time.Now())
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (s *JobStore) finish(id string, result any, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return
	}
	now := time.Now()
	job.UpdatedAt = now
	job.FinishedAt = &now
	if err != nil {
		job.Status = JobFailed
		job.Error = err.Error()
		job.Result = nil
	} else {
		job.Status = JobSucceeded
		job.Result = result
	}
	s.pruneLocked(now)
}

func (s *JobStore) pruneLocked(now time.Time) {
	// Mark queued/paused jobs as expired when TTL exceeded.
	for _, job := range s.jobs {
		if now.Sub(job.CreatedAt) <= s.ttl {
			continue
		}
		switch job.Status {
		case JobQueued, JobPaused:
			job.Status = JobExpired
			job.Error = "job expired"
			job.UpdatedAt = now
			job.FinishedAt = &now
		}
	}

	// Remove terminal jobs past retention TTL.
	for id, job := range s.jobs {
		if !isTerminal(job.Status) || job.FinishedAt == nil {
			continue
		}
		if now.Sub(*job.FinishedAt) > s.ttl {
			delete(s.jobs, id)
		}
	}

	// Enforce max size by evicting oldest terminal jobs.
	if s.max <= 0 || len(s.jobs) <= s.max {
		return
	}
	for len(s.jobs) > s.max {
		oldestID := ""
		var oldest time.Time
		for id, job := range s.jobs {
			if !isTerminal(job.Status) || job.FinishedAt == nil {
				continue
			}
			if oldestID == "" || job.CreatedAt.Before(oldest) {
				oldestID = id
				oldest = job.CreatedAt
			}
		}
		if oldestID == "" {
			break
		}
		delete(s.jobs, oldestID)
	}
}

func isTerminal(status JobStatus) bool {
	switch status {
	case JobSucceeded, JobFailed, JobCanceled, JobExpired:
		return true
	default:
		return false
	}
}

func cloneJob(job *Job) *Job {
	if job == nil {
		return nil
	}
	copy := *job
	return &copy
}

func newJobID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "job-" + hex.EncodeToString(b)
}
