package domain

import (
	"encoding/json"
	"time"
)

type JobState string

const (
	StateAvailable JobState = "available"
	StateCancelled JobState = "cancelled"
	StateCompleted JobState = "completed"
	StateDiscarded JobState = "discarded"
	StatePending   JobState = "pending"
	StateRetryable JobState = "retryable"
	StateRunning   JobState = "running"
	StateScheduled JobState = "scheduled"
)

var ListableStates = map[string]JobState{
	"available": StateAvailable,
	"cancelled": StateCancelled,
	"completed": StateCompleted,
	"discarded": StateDiscarded,
	"pending":   StatePending,
	"retryable": StateRetryable,
	"running":   StateRunning,
	"scheduled": StateScheduled,
}

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
)

type ScrapeJob struct {
	ID      int64
	Attempt int
	Params  ScrapeParams
}

type JobRecord struct {
	ID          int64
	State       JobState
	Keyword     string
	CreatedAt   time.Time
	AttemptedAt *time.Time
	FinalizedAt *time.Time
	LastError   string
}

type JobListFilter struct {
	State  JobState
	Limit  int
	Cursor string
}

type JobPage struct {
	Records    []JobRecord
	NextCursor string
}

type ScrapeResult struct {
	JobID       int64
	Keyword     string
	Results     json.RawMessage
	ResultCount int
}

type FlushResult struct {
	ResultCount int
	Err         error
}
