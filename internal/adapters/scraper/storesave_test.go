package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OBrunoW/gmaps-scraper/internal/adapters/gmaps"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeResultRepository struct {
	domain.ResultRepository
	saved []domain.ScrapeResult
	err   error
}

func (f *fakeResultRepository) Save(_ context.Context, result domain.ScrapeResult) error {
	f.saved = append(f.saved, result)

	return f.err
}

func TestStoreSavePassesMarshaledEntriesToRepository(t *testing.T) {
	repo := &fakeResultRepository{}
	entries := []*gmaps.Entry{{Title: "Cafe A"}, {Title: "Cafe B"}}

	err := storeSave(repo)(context.Background(), 42, "coffee", entries)
	require.NoError(t, err)
	require.Len(t, repo.saved, 1)

	expectedJSON, err := json.Marshal(entries)
	require.NoError(t, err)

	saved := repo.saved[0]
	require.Equal(t, int64(42), saved.JobID)
	require.Equal(t, "coffee", saved.Keyword)
	require.Equal(t, 2, saved.ResultCount)
	require.JSONEq(t, string(expectedJSON), string(saved.Results))
}

func TestStoreSaveReturnsRepositoryError(t *testing.T) {
	repo := &fakeResultRepository{err: errors.New("db down")}

	err := storeSave(repo)(context.Background(), 1, "k", nil)
	require.EqualError(t, err, "db down")
}

func TestBuildScrapeJobDefaultsMaxDepthAndParsesGeo(t *testing.T) {
	job := buildScrapeJob("7", domain.ScrapeParams{Keyword: "k", Lang: "en"}, nil)
	gmapJob, ok := job.(*gmaps.GmapJob)
	require.True(t, ok)
	require.Equal(t, 10, gmapJob.MaxDepth)
	require.True(t, gmapJob.WriterManagedCompletion)

	lat, lon := parseGeoCoordinates("-23.5, 46.6")
	require.InDelta(t, -23.5, lat, 0.0001)
	require.InDelta(t, 46.6, lon, 0.0001)
}

func TestBuildScrapeJobFastModeBuildsSearchJob(t *testing.T) {
	job := buildScrapeJob("7", domain.ScrapeParams{Keyword: "k", Lang: "en", FastMode: true, GeoCoordinates: "1,2", Zoom: 10}, nil)
	searchJob, ok := job.(*gmaps.SearchJob)
	require.True(t, ok)
	require.Equal(t, "7", searchJob.ID)
	require.True(t, searchJob.WriterManagedCompletion)
}
