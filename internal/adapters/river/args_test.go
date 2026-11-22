package river

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
)

const legacyScrapeArgs = `{"keyword":"coffee","lang":"pt","max_depth":2,"email":true,"geo_coordinates":"-23.5,-46.6","zoom":15,"radius":500.5,"fast_mode":true,"extra_reviews":true,"timeout":120}`

func TestScrapeJobArgsDecodesLegacyJSON(t *testing.T) {
	var args ScrapeJobArgs
	require.NoError(t, json.Unmarshal([]byte(legacyScrapeArgs), &args))

	require.Equal(t, ScrapeJobArgs{
		Keyword:        "coffee",
		Lang:           "pt",
		MaxDepth:       2,
		Email:          true,
		GeoCoordinates: "-23.5,-46.6",
		Zoom:           15,
		Radius:         500.5,
		FastMode:       true,
		ExtraReviews:   true,
		TimeoutSecs:    120,
	}, args)
}

func TestScrapeJobArgsEncodesLegacyJSON(t *testing.T) {
	var args ScrapeJobArgs
	require.NoError(t, json.Unmarshal([]byte(legacyScrapeArgs), &args))

	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	require.JSONEq(t, legacyScrapeArgs, string(encoded))
}

func TestJobDeleteArgsJSON(t *testing.T) {
	var args JobDeleteArgs
	require.NoError(t, json.Unmarshal([]byte(`{"job_id":42}`), &args))
	require.Equal(t, int64(42), args.JobID)

	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	require.JSONEq(t, `{"job_id":42}`, string(encoded))
}

func TestArgsKindsAndInsertOpts(t *testing.T) {
	require.Equal(t, "scrape", ScrapeJobArgs{}.Kind())
	require.Equal(t, "job_delete", JobDeleteArgs{}.Kind())
	require.Equal(t, "cleanup", CleanupArgs{}.Kind())

	require.Equal(t, river.InsertOpts{MaxAttempts: 3}, ScrapeJobArgs{}.InsertOpts())
	require.Equal(t, river.InsertOpts{Queue: "maintenance", MaxAttempts: 3}, JobDeleteArgs{}.InsertOpts())
	require.Equal(t, river.InsertOpts{Queue: "maintenance", MaxAttempts: 1}, CleanupArgs{}.InsertOpts())
}

func TestArgsParamsRoundTrip(t *testing.T) {
	params := domain.ScrapeParams{
		Keyword: "pizza", Lang: "en", MaxDepth: 3, Email: true, GeoCoordinates: "1,2",
		Zoom: 10, Radius: 1.5, FastMode: true, ExtraReviews: true, TimeoutSecs: 60,
	}

	require.Equal(t, params, argsFromParams(params).params())
}

func TestScrapeWorkerTimeout(t *testing.T) {
	worker := &ScrapeWorker{}

	tests := []struct {
		name string
		job  *river.Job[ScrapeJobArgs]
		want time.Duration
	}{
		{"nil job", nil, 7 * time.Minute},
		{"zero timeout", &river.Job[ScrapeJobArgs]{Args: ScrapeJobArgs{}}, 7 * time.Minute},
		{"short timeout", &river.Job[ScrapeJobArgs]{Args: ScrapeJobArgs{TimeoutSecs: 60}}, 3 * time.Minute},
		{"capped timeout", &river.Job[ScrapeJobArgs]{Args: ScrapeJobArgs{TimeoutSecs: 900}}, 7 * time.Minute},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, worker.Timeout(test.job))
		})
	}
}

func TestScrapeWorkerNextRetryAt(t *testing.T) {
	before := time.Now()
	next := (&ScrapeWorker{}).NextRetryAt(nil)

	require.WithinDuration(t, before.Add(2*time.Minute), next, time.Second)
}

func TestScrapeWorkerWorkWithoutRunner(t *testing.T) {
	err := (&ScrapeWorker{}).Work(t.Context(), &river.Job[ScrapeJobArgs]{JobRow: &rivertype.JobRow{ID: 1}})
	require.ErrorIs(t, err, errRunnerNotConfigured)
}

type fakeMaintenance struct {
	purged  []int64
	cleaned int
}

func (f *fakeMaintenance) Purge(_ context.Context, id int64) error {
	f.purged = append(f.purged, id)
	return nil
}

func (f *fakeMaintenance) CleanupExpiredResults(_ context.Context) error {
	f.cleaned++
	return nil
}

func TestMaintenanceWorkersDelegate(t *testing.T) {
	service := &fakeMaintenance{}
	ref := &maintenanceRef{service: service}

	deleteJob := &river.Job[JobDeleteArgs]{JobRow: &rivertype.JobRow{ID: 9}, Args: JobDeleteArgs{JobID: 7}}
	require.NoError(t, (&JobDeleteWorker{ref: ref}).Work(t.Context(), deleteJob))
	require.Equal(t, []int64{7}, service.purged)

	require.NoError(t, (&CleanupWorker{ref: ref}).Work(t.Context(), &river.Job[CleanupArgs]{JobRow: &rivertype.JobRow{ID: 10}}))
	require.Equal(t, 1, service.cleaned)
}

func TestMaintenanceWorkersWithoutService(t *testing.T) {
	ref := &maintenanceRef{}

	err := (&JobDeleteWorker{ref: ref}).Work(t.Context(), &river.Job[JobDeleteArgs]{JobRow: &rivertype.JobRow{ID: 1}})
	require.ErrorIs(t, err, errMaintenanceNotConfigured)

	err = (&CleanupWorker{ref: ref}).Work(t.Context(), &river.Job[CleanupArgs]{JobRow: &rivertype.JobRow{ID: 1}})
	require.ErrorIs(t, err, errMaintenanceNotConfigured)
}

func TestToRecord(t *testing.T) {
	created := time.Now()
	job := &rivertype.JobRow{
		ID:          5,
		State:       rivertype.JobStateDiscarded,
		CreatedAt:   created,
		EncodedArgs: []byte(legacyScrapeArgs),
		Errors:      []rivertype.AttemptError{{Error: "first"}, {Error: "last"}},
	}

	record := toRecord(job)
	require.Equal(t, int64(5), record.ID)
	require.Equal(t, domain.StateDiscarded, record.State)
	require.Equal(t, "coffee", record.Keyword)
	require.Equal(t, "last", record.LastError)
	require.Equal(t, created, record.CreatedAt)
}
