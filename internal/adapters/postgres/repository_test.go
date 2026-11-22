package postgres

import (
	"context"
	"testing"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

var (
	_ domain.ResultRepository = (*ResultRepository)(nil)
	_ domain.JobRepository    = (*JobRepository)(nil)
)

func TestNewRepositoriesKeepPool(t *testing.T) {
	require.Nil(t, NewResultRepository(nil).pool)
	require.Nil(t, NewJobRepository(nil).pool)
}

func TestCountByJobIDsEmptySkipsDatabase(t *testing.T) {
	counts, err := NewResultRepository(nil).CountByJobIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, counts)
}
