package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
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
	schema := fmt.Sprintf("adapter_test_%d", time.Now().UnixNano())

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

func insertCompletedJob(t *testing.T, pool *pgxpool.Pool, finishedAgo time.Duration) int64 {
	t.Helper()

	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO river_job (state, max_attempts, kind, args, finalized_at)
		VALUES ('completed', 3, 'scrape', '{}', NOW() - make_interval(secs => $1))
		RETURNING id`, finishedAgo.Seconds()).Scan(&id)
	require.NoError(t, err)

	return id
}

func TestResultRepositorySaveUpsert(t *testing.T) {
	pool := newTestPool(t)
	repository := NewResultRepository(pool)
	ctx := context.Background()

	first := domain.ScrapeResult{JobID: 1, Keyword: "coffee", Results: json.RawMessage(`[{"a":1}]`), ResultCount: 1}
	require.NoError(t, repository.Save(ctx, first))

	second := domain.ScrapeResult{JobID: 1, Keyword: "coffee", Results: json.RawMessage(`[{"a":1},{"a":2}]`), ResultCount: 2}
	require.NoError(t, repository.Save(ctx, second))

	found, err := repository.FindByJobID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 2, found.ResultCount)
	require.JSONEq(t, `[{"a":1},{"a":2}]`, string(found.Results))
}

func TestResultRepositoryFindByJobIDMissing(t *testing.T) {
	pool := newTestPool(t)
	repository := NewResultRepository(pool)

	_, err := repository.FindByJobID(context.Background(), 404)
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestResultRepositoryCountByJobIDs(t *testing.T) {
	pool := newTestPool(t)
	repository := NewResultRepository(pool)
	ctx := context.Background()

	require.NoError(t, repository.Save(ctx, domain.ScrapeResult{JobID: 1, Keyword: "a", Results: json.RawMessage(`[]`), ResultCount: 3}))
	require.NoError(t, repository.Save(ctx, domain.ScrapeResult{JobID: 2, Keyword: "b", Results: json.RawMessage(`[]`), ResultCount: 5}))
	require.NoError(t, repository.Save(ctx, domain.ScrapeResult{JobID: 3, Keyword: "c", Results: json.RawMessage(`[]`), ResultCount: 7}))

	counts, err := repository.CountByJobIDs(ctx, []int64{1, 2, 99})
	require.NoError(t, err)
	require.Equal(t, map[int64]int{1: 3, 2: 5}, counts)

	empty, err := repository.CountByJobIDs(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestResultRepositoryDeleteByJobID(t *testing.T) {
	pool := newTestPool(t)
	repository := NewResultRepository(pool)
	ctx := context.Background()

	require.NoError(t, repository.Save(ctx, domain.ScrapeResult{JobID: 1, Keyword: "a", Results: json.RawMessage(`[]`)}))

	affected, err := repository.DeleteByJobID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)

	affected, err = repository.DeleteByJobID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, int64(0), affected)
}

func TestResultRepositoryDeleteExpired(t *testing.T) {
	pool := newTestPool(t)
	repository := NewResultRepository(pool)
	ctx := context.Background()

	oldJobID := insertCompletedJob(t, pool, 31*24*time.Hour)
	recentJobID := insertCompletedJob(t, pool, time.Hour)

	require.NoError(t, repository.Save(ctx, domain.ScrapeResult{JobID: oldJobID, Keyword: "old", Results: json.RawMessage(`[]`)}))
	require.NoError(t, repository.Save(ctx, domain.ScrapeResult{JobID: recentJobID, Keyword: "recent", Results: json.RawMessage(`[]`)}))

	affected, err := repository.DeleteExpired(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)

	_, err = repository.FindByJobID(ctx, oldJobID)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = repository.FindByJobID(ctx, recentJobID)
	require.NoError(t, err)
}

func TestJobRepositoryDelete(t *testing.T) {
	pool := newTestPool(t)
	repository := NewJobRepository(pool)
	ctx := context.Background()

	jobID := insertCompletedJob(t, pool, time.Hour)

	affected, err := repository.Delete(ctx, jobID)
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)

	affected, err = repository.Delete(ctx, jobID)
	require.NoError(t, err)
	require.Equal(t, int64(0), affected)
}
