package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeScrapeJobs struct {
	created   []domain.ScrapeParams
	createErr error
	view      JobView
	getErr    error
}

func (f *fakeScrapeJobs) Create(_ context.Context, params domain.ScrapeParams) (CreatedJob, error) {
	f.created = append(f.created, params)
	if f.createErr != nil {
		return CreatedJob{}, f.createErr
	}

	return CreatedJob{PublicID: "abc123", Status: domain.StatusPending}, nil
}

func (f *fakeScrapeJobs) Get(_ context.Context, _ string) (JobView, error) {
	return f.view, f.getErr
}

type fakeGeocoder struct {
	point          domain.GeoPoint
	err            error
	cityCalls      []string
	addressCalls   []domain.Address
	addressCity    string
	addressUF      string
	cityUF         string
	totalCallCount int
}

func (f *fakeGeocoder) GeocodeCity(_ context.Context, city, uf string) (domain.GeoPoint, error) {
	f.totalCallCount++
	f.cityCalls = append(f.cityCalls, city)
	f.cityUF = uf

	return f.point, f.err
}

func (f *fakeGeocoder) GeocodeAddress(_ context.Context, address domain.Address, city, uf string) (domain.GeoPoint, error) {
	f.totalCallCount++
	f.addressCalls = append(f.addressCalls, address)
	f.addressCity = city
	f.addressUF = uf

	return f.point, f.err
}

func prospectorInput() domain.ProspectorParams {
	return domain.ProspectorParams{Keyword: " padarias ", City: " Curitiba ", UF: "pr"}
}

func TestProspectorCreateWithCity(t *testing.T) {
	jobs := &fakeScrapeJobs{}
	geocoder := &fakeGeocoder{point: domain.GeoPoint{Lat: -25.4284, Lng: -49.2733}}
	service := NewProspectorService(jobs, geocoder)

	created, err := service.Create(context.Background(), prospectorInput())

	require.NoError(t, err)
	require.Equal(t, CreatedProspectorJob{
		PublicID: "abc123",
		Status:   domain.StatusPending,
		Keyword:  "padarias",
		City:     "Curitiba",
		RadiusKm: 5,
	}, created)
	require.Equal(t, []string{"Curitiba"}, geocoder.cityCalls)
	require.Equal(t, "PR", geocoder.cityUF)
	require.Empty(t, geocoder.addressCalls)
	require.Equal(t, []domain.ScrapeParams{{
		Keyword:        "padarias em Curitiba",
		Lang:           "pt",
		MaxDepth:       1,
		GeoCoordinates: "-25.4284,-49.2733",
		Zoom:           14,
		Radius:         5000,
		FastMode:       true,
		TimeoutSecs:    300,
	}}, jobs.created)
}

func TestProspectorCreateWithAddress(t *testing.T) {
	jobs := &fakeScrapeJobs{}
	geocoder := &fakeGeocoder{point: domain.GeoPoint{Lat: -25.5, Lng: -49.3}}
	service := NewProspectorService(jobs, geocoder)

	input := prospectorInput()
	input.Address = &domain.Address{Street: " Rua XV ", Number: "100"}

	_, err := service.Create(context.Background(), input)

	require.NoError(t, err)
	require.Empty(t, geocoder.cityCalls)
	require.Equal(t, []domain.Address{{Street: "Rua XV", Number: "100"}}, geocoder.addressCalls)
	require.Equal(t, "Curitiba", geocoder.addressCity)
	require.Equal(t, "PR", geocoder.addressUF)
	require.Equal(t, "-25.5,-49.3", jobs.created[0].GeoCoordinates)
}

func TestProspectorCreateAddressWithoutUF(t *testing.T) {
	jobs := &fakeScrapeJobs{}
	geocoder := &fakeGeocoder{}
	service := NewProspectorService(jobs, geocoder)

	input := prospectorInput()
	input.UF = ""
	input.Address = &domain.Address{Street: "Rua XV"}

	_, err := service.Create(context.Background(), input)

	requireAppError(t, err, http.StatusBadRequest, domain.CodeInvalidInput, "uf e obrigatoria quando address e informado")
	require.Zero(t, geocoder.totalCallCount)
	require.Empty(t, jobs.created)
}

func TestProspectorCreateValidationSkipsGeocoder(t *testing.T) {
	jobs := &fakeScrapeJobs{}
	geocoder := &fakeGeocoder{}
	service := NewProspectorService(jobs, geocoder)

	cases := []struct {
		name    string
		mutate  func(*domain.ProspectorParams)
		message string
	}{
		{"keyword", func(p *domain.ProspectorParams) { p.Keyword = "a" }, "keyword deve ter entre 2 e 120 caracteres"},
		{"city", func(p *domain.ProspectorParams) { p.City = " " }, "city deve ter entre 2 e 80 caracteres"},
		{"uf", func(p *domain.ProspectorParams) { p.UF = "PRR" }, "uf deve ter 2 letras"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := prospectorInput()
			tc.mutate(&input)

			_, err := service.Create(context.Background(), input)

			requireAppError(t, err, http.StatusBadRequest, domain.CodeInvalidInput, tc.message)
		})
	}

	require.Zero(t, geocoder.totalCallCount)
	require.Empty(t, jobs.created)
}

func TestProspectorCreateRadius(t *testing.T) {
	cases := []struct {
		name       string
		radius     *float64
		wantKm     int
		wantMeters float64
	}{
		{"absent", nil, 5, 5000},
		{"zero", floatPointer(0), 1, 1000},
		{"rounds", floatPointer(7.6), 8, 8000},
		{"clamped high", floatPointer(80), 50, 50000},
		{"clamped low", floatPointer(-4), 1, 1000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jobs := &fakeScrapeJobs{}
			service := NewProspectorService(jobs, &fakeGeocoder{point: domain.GeoPoint{Lat: 1, Lng: 2}})

			input := prospectorInput()
			input.RadiusKm = tc.radius

			created, err := service.Create(context.Background(), input)

			require.NoError(t, err)
			require.Equal(t, tc.wantKm, created.RadiusKm)
			require.Equal(t, tc.wantMeters, jobs.created[0].Radius)
			require.True(t, jobs.created[0].FastMode)
		})
	}
}

func TestProspectorCreateGeocodingErrors(t *testing.T) {
	withAddress := func(p *domain.ProspectorParams) { p.Address = &domain.Address{Street: "Rua XV"} }
	noChange := func(*domain.ProspectorParams) {}

	cases := []struct {
		name    string
		mutate  func(*domain.ProspectorParams)
		err     error
		status  int
		code    string
		message string
	}{
		{"city not found", noChange, domain.ErrLocationNotFound, http.StatusBadRequest, domain.CodeCityNotFound, "Cidade nao encontrada"},
		{"address not found", withAddress, fmt.Errorf("lookup: %w", domain.ErrLocationNotFound), http.StatusBadRequest, domain.CodeAddressNotFound, "Endereco nao encontrado"},
		{"city unavailable", noChange, fmt.Errorf("status 429: %w", domain.ErrGeocoderUnavailable), http.StatusBadGateway, domain.CodeGeocoderUnavailable, "Servico de geocodificacao indisponivel"},
		{"address unavailable", withAddress, domain.ErrGeocoderUnavailable, http.StatusBadGateway, domain.CodeGeocoderUnavailable, "Servico de geocodificacao indisponivel"},
		{"unknown error", noChange, errors.New("boom"), http.StatusBadGateway, domain.CodeGeocoderUnavailable, "Servico de geocodificacao indisponivel"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jobs := &fakeScrapeJobs{}
			service := NewProspectorService(jobs, &fakeGeocoder{err: tc.err})

			input := prospectorInput()
			tc.mutate(&input)

			_, err := service.Create(context.Background(), input)

			requireAppError(t, err, tc.status, tc.code, tc.message)
			require.Empty(t, jobs.created)
		})
	}
}

func TestProspectorCreateAddressNeverFallsBackToCity(t *testing.T) {
	geocoder := &fakeGeocoder{err: domain.ErrLocationNotFound}
	service := NewProspectorService(&fakeScrapeJobs{}, geocoder)

	input := prospectorInput()
	input.Address = &domain.Address{Street: "Rua XV"}

	_, err := service.Create(context.Background(), input)

	requireAppError(t, err, http.StatusBadRequest, domain.CodeAddressNotFound, "Endereco nao encontrado")
	require.Empty(t, geocoder.cityCalls)
}

func TestProspectorCreatePropagatesJobError(t *testing.T) {
	jobErr := domain.Internal("failed to create job", errors.New("db down"))
	service := NewProspectorService(&fakeScrapeJobs{createErr: jobErr}, &fakeGeocoder{point: domain.GeoPoint{Lat: 1, Lng: 2}})

	_, err := service.Create(context.Background(), prospectorInput())

	require.Same(t, jobErr, err)
}

func TestProspectorGetPendingHasNoLeads(t *testing.T) {
	service := NewProspectorService(&fakeScrapeJobs{view: JobView{
		PublicID: "abc123",
		Status:   domain.StatusRunning,
		Keyword:  "padarias em Curitiba",
	}}, &fakeGeocoder{})

	job, err := service.Get(context.Background(), "abc123")

	require.NoError(t, err)
	require.Equal(t, domain.ProspectorJob{PublicID: "abc123", Status: domain.StatusRunning, Keyword: "padarias em Curitiba"}, job)
	require.Nil(t, job.Leads)
}

func TestProspectorGetFailedCarriesError(t *testing.T) {
	service := NewProspectorService(&fakeScrapeJobs{view: JobView{
		PublicID: "abc123",
		Status:   domain.StatusFailed,
		Error:    "job timed out with no results",
	}}, &fakeGeocoder{})

	job, err := service.Get(context.Background(), "abc123")

	require.NoError(t, err)
	require.Equal(t, "job timed out with no results", job.Error)
	require.Nil(t, job.Leads)
}

func TestProspectorGetCompletedNormalizesLeads(t *testing.T) {
	service := NewProspectorService(&fakeScrapeJobs{view: JobView{
		PublicID:    "abc123",
		Status:      domain.StatusCompleted,
		Keyword:     "padarias em Curitiba",
		ResultCount: 3,
		Results: json.RawMessage(`[
			{"place_id":"p1","title":"Padaria Central","latitude":-25.4,"longitude":-49.2},
			{"place_id":"p1","title":"Duplicate"},
			{"title":"No key"}
		]`),
	}}, &fakeGeocoder{})

	job, err := service.Get(context.Background(), "abc123")

	require.NoError(t, err)
	require.Equal(t, 3, job.ResultCount)
	require.Len(t, job.Leads, 1)
	require.Equal(t, "place:p1", job.Leads[0].Key)
}

func TestProspectorGetCompletedWithoutResultsReturnsEmptySlice(t *testing.T) {
	service := NewProspectorService(&fakeScrapeJobs{view: JobView{
		PublicID: "abc123",
		Status:   domain.StatusCompleted,
	}}, &fakeGeocoder{})

	job, err := service.Get(context.Background(), "abc123")

	require.NoError(t, err)
	require.NotNil(t, job.Leads)
	require.Empty(t, job.Leads)
}

func TestProspectorGetInvalidResultsIsInternalError(t *testing.T) {
	service := NewProspectorService(&fakeScrapeJobs{view: JobView{
		Status:  domain.StatusCompleted,
		Results: json.RawMessage(`[{"place_id":`),
	}}, &fakeGeocoder{})

	_, err := service.Get(context.Background(), "abc123")

	requireAppError(t, err, http.StatusInternalServerError, domain.CodeInternal, "failed to read job results")
}

func TestProspectorGetPropagatesNotFound(t *testing.T) {
	notFound := domain.NotFound("job not found")
	service := NewProspectorService(&fakeScrapeJobs{getErr: notFound}, &fakeGeocoder{})

	_, err := service.Get(context.Background(), "missing")

	require.Same(t, notFound, err)
}
