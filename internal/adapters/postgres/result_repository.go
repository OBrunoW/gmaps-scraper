package postgres

import (
	"context"
	"fmt"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ResultRepository struct {
	pool *pgxpool.Pool
}

func NewResultRepository(pool *pgxpool.Pool) *ResultRepository {
	return &ResultRepository{pool: pool}
}

func (r *ResultRepository) Save(ctx context.Context, result domain.ScrapeResult) error {
	q := `INSERT INTO scrape_results (job_id, keyword, results, result_count)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (job_id) DO UPDATE SET
				results = $3,
				result_count = $4`

	if _, err := r.pool.Exec(ctx, q, result.JobID, result.Keyword, result.Results, result.ResultCount); err != nil {
		return fmt.Errorf("save scrape result for job %d: %w", result.JobID, err)
	}

	return nil
}

func (r *ResultRepository) FindByJobID(ctx context.Context, id int64) (domain.ScrapeResult, error) {
	q := `SELECT results, result_count FROM scrape_results WHERE job_id = $1`

	result := domain.ScrapeResult{JobID: id}
	if err := r.pool.QueryRow(ctx, q, id).Scan(&result.Results, &result.ResultCount); err != nil {
		return domain.ScrapeResult{}, fmt.Errorf("find scrape result for job %d: %w", id, err)
	}

	return result, nil
}

func (r *ResultRepository) CountByJobIDs(ctx context.Context, ids []int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	if len(ids) == 0 {
		return counts, nil
	}

	q := `SELECT job_id, result_count FROM scrape_results WHERE job_id = ANY($1)`

	rows, err := r.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("count scrape results: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var jobID int64
		var count int
		if err := rows.Scan(&jobID, &count); err != nil {
			return nil, fmt.Errorf("scan scrape result count: %w", err)
		}
		counts[jobID] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scrape result counts: %w", err)
	}

	return counts, nil
}

func (r *ResultRepository) DeleteByJobID(ctx context.Context, id int64) (int64, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM scrape_results WHERE job_id = $1", id)
	if err != nil {
		return 0, fmt.Errorf("delete scrape result for job %d: %w", id, err)
	}

	return tag.RowsAffected(), nil
}

func (r *ResultRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM scrape_results WHERE job_id IN (
			SELECT id FROM river_job WHERE state = 'completed' AND finalized_at < NOW() - INTERVAL '30 days'
		)`)
	if err != nil {
		return 0, fmt.Errorf("delete expired scrape results: %w", err)
	}

	return tag.RowsAffected(), nil
}
