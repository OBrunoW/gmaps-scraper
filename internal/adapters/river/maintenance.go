package river

import (
	"context"
	"errors"

	"github.com/riverqueue/river"
)

var errMaintenanceNotConfigured = errors.New("maintenance service is not configured")

type Maintenance interface {
	Purge(ctx context.Context, id int64) error
	CleanupExpiredResults(ctx context.Context) error
}

type maintenanceRef struct {
	service Maintenance
}

type JobDeleteWorker struct {
	river.WorkerDefaults[JobDeleteArgs]
	ref *maintenanceRef
}

func (w *JobDeleteWorker) Work(ctx context.Context, job *river.Job[JobDeleteArgs]) error {
	if w.ref.service == nil {
		return errMaintenanceNotConfigured
	}

	return w.ref.service.Purge(ctx, job.Args.JobID)
}

type CleanupWorker struct {
	river.WorkerDefaults[CleanupArgs]
	ref *maintenanceRef
}

func (w *CleanupWorker) Work(ctx context.Context, _ *river.Job[CleanupArgs]) error {
	if w.ref.service == nil {
		return errMaintenanceNotConfigured
	}

	return w.ref.service.CleanupExpiredResults(ctx)
}
