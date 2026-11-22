package river

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("river_test_%d", time.Now().UnixNano())

	adminConn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)

	_, err = adminConn.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = adminConn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = adminConn.Close(ctx)
	})

	poolConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	migrationDB := sql.OpenDB(stdlib.GetConnector(*poolConfig.ConnConfig))
	t.Cleanup(func() { _ = migrationDB.Close() })

	_, err = migrations.Run(migrationDB)
	require.NoError(t, err)

	return pool
}

func newTestQueue(t *testing.T) (*Queue, *pgxpool.Pool) {
	t.Helper()

	pool := newTestPool(t)
	client, err := NewClient(pool)
	require.NoError(t, err)

	return client.Queue(), pool
}

func sampleParams(keyword string) domain.ScrapeParams {
	return domain.ScrapeParams{Keyword: keyword, Lang: "en", MaxDepth: 1, TimeoutSecs: 300}
}

func TestQueueEnqueueOnServeClient(t *testing.T) {
	queue, _ := newTestQueue(t)
	ctx := context.Background()

	id, err := queue.Enqueue(ctx, sampleParams("coffee"))
	require.NoError(t, err)
	require.Positive(t, id)

	record, err := queue.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, id, record.ID)
	require.Equal(t, "coffee", record.Keyword)
	require.Equal(t, domain.StateAvailable, record.State)
}

func TestQueueGetMissing(t *testing.T) {
	queue, _ := newTestQueue(t)

	_, err := queue.Get(context.Background(), 999999)
	require.Error(t, err)
}

func TestQueueListByStateAndCursor(t *testing.T) {
	queue, _ := newTestQueue(t)
	ctx := context.Background()

	for _, keyword := range []string{"a", "b", "c"} {
		_, err := queue.Enqueue(ctx, sampleParams(keyword))
		require.NoError(t, err)
	}

	first, err := queue.List(ctx, domain.JobListFilter{State: domain.StateAvailable, Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Records, 2)
	require.NotEmpty(t, first.NextCursor)

	second, err := queue.List(ctx, domain.JobListFilter{State: domain.StateAvailable, Limit: 2, Cursor: first.NextCursor})
	require.NoError(t, err)
	require.Len(t, second.Records, 1)

	none, err := queue.List(ctx, domain.JobListFilter{State: domain.StateCompleted, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, none.Records)
}

func TestQueueListInvalidCursor(t *testing.T) {
	queue, _ := newTestQueue(t)

	_, err := queue.List(context.Background(), domain.JobListFilter{Limit: 5, Cursor: "not-a-cursor"})
	require.Error(t, err)

	var appErr *domain.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, domain.CodeInvalidInput, appErr.Code)
	require.Contains(t, appErr.Message, "invalid cursor: ")
}

func TestQueueCancel(t *testing.T) {
	queue, _ := newTestQueue(t)
	ctx := context.Background()

	id, err := queue.Enqueue(ctx, sampleParams("coffee"))
	require.NoError(t, err)

	require.NoError(t, queue.Cancel(ctx, id))

	record, err := queue.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, domain.StateCancelled, record.State)
}

func TestQueueEnqueuePurge(t *testing.T) {
	queue, pool := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, queue.EnqueuePurge(ctx, 123))

	var queueName string
	var count int
	err := pool.QueryRow(ctx,
		`SELECT queue, count(*) OVER () FROM river_job WHERE kind = 'job_delete' AND (args->>'job_id')::bigint = 123`).
		Scan(&queueName, &count)
	require.NoError(t, err)
	require.Equal(t, "maintenance", queueName)
	require.Equal(t, 1, count)
}
