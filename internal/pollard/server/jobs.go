package server

import (
	"time"

	"github.com/mistakeknot/autarch/pkg/jobs"
)

type JobStatus = jobs.JobStatus

const (
	JobQueued    = jobs.JobQueued
	JobRunning   = jobs.JobRunning
	JobSucceeded = jobs.JobSucceeded
	JobFailed    = jobs.JobFailed
	JobCanceled  = jobs.JobCanceled
	JobExpired   = jobs.JobExpired
	JobStalled   = jobs.JobStalled
	JobRetrying  = jobs.JobRetrying
	JobPaused    = jobs.JobPaused
)

type Job = jobs.Job
type JobStore = jobs.JobStore

func NewJobStore(ttl time.Duration, max int) *jobs.JobStore {
	return jobs.NewJobStore(ttl, max)
}
