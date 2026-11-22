package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type JobRepository struct {
	pool *pgxpool.Pool
}

func NewJobRepository(pool *pgxpool.Pool) *JobRepository {
	return &JobRepository{pool: pool}
}

func (r *JobRepository) Delete(ctx context.Context, id int64) (int64, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM river_job WHERE id = $1", id)
	if err != nil {
		return 0, fmt.Errorf("delete job %d: %w", id, err)
	}

	return tag.RowsAffected(), nil
}
