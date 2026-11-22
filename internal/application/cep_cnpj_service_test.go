package application

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeCEPClient struct {
	location domain.CEPLocation
	err      error
	received string
}

func (f *fakeCEPClient) LookupCEP(_ context.Context, cep string) (domain.CEPLocation, error) {
	f.received = cep
	return f.location, f.err
}

type fakeCNPJClient struct {
	company  domain.CNPJCompany
	err      error
	received string
	calls    int
}

func (f *fakeCNPJClient) LookupCNPJ(_ context.Context, cnpj string) (domain.CNPJCompany, error) {
	f.calls++
	f.received = cnpj
	return f.company, f.err
}

func requireAppErrorCode(t *testing.T, err error, status int, code string) {
	t.Helper()

	var appErr *domain.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, status, appErr.Status)
	require.Equal(t, code, appErr.Code)
}

func TestCEPServiceNormalizesInput(t *testing.T) {
	client := &fakeCEPClient{location: domain.CEPLocation{City: "Curitiba"}}

	location, err := NewCEPService(client).Lookup(context.Background(), "80010-000")

	require.NoError(t, err)
	require.Equal(t, "80010000", client.received)
	require.Equal(t, "Curitiba", location.City)
}

func TestCEPServiceRejectsWrongLength(t *testing.T) {
	client := &fakeCEPClient{}

	_, err := NewCEPService(client).Lookup(context.Background(), "8001000")

	requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeInvalidInput)
	require.Empty(t, client.received)
}

func TestCEPServiceMapsNotFound(t *testing.T) {
	client := &fakeCEPClient{err: domain.ErrCEPNotFound}

	_, err := NewCEPService(client).Lookup(context.Background(), "80010000")

	requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeCEPNotFound)
}

func TestCEPServiceMapsUnavailable(t *testing.T) {
	client := &fakeCEPClient{err: errors.Join(domain.ErrGeocoderUnavailable, errors.New("boom"))}

	_, err := NewCEPService(client).Lookup(context.Background(), "80010000")

	requireAppErrorCode(t, err, http.StatusBadGateway, domain.CodeGeocoderUnavailable)
}

func TestCNPJServiceValidatesDigits(t *testing.T) {
	cases := map[string]struct {
		input string
		valid bool
	}{
		"valid":        {"11222333000181", true},
		"punctuation":  {"11.222.333/0001-81", true},
		"wrong check":  {"11222333000182", false},
		"wrong length": {"1122233300018", false},
		"all same":     {"11111111111111", false},
		"empty":        {"", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client := &fakeCNPJClient{company: domain.CNPJCompany{LegalName: "X"}}

			_, err := NewCNPJService(client).Lookup(context.Background(), tc.input)

			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, "11222333000181", client.received)
				return
			}

			requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeInvalidCNPJ)
			require.Zero(t, client.calls)
		})
	}
}

func TestCNPJServiceMapsClientErrors(t *testing.T) {
	invalid := &fakeCNPJClient{err: domain.ErrInvalidCNPJ}
	_, err := NewCNPJService(invalid).Lookup(context.Background(), "11222333000181")
	requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeInvalidCNPJ)

	unavailable := &fakeCNPJClient{err: errors.New("boom")}
	_, err = NewCNPJService(unavailable).Lookup(context.Background(), "11222333000181")
	requireAppErrorCode(t, err, http.StatusBadGateway, domain.CodeUpstreamUnavailable)
}

func TestProspectorCreateWithCEP(t *testing.T) {
	latitude, longitude := -25.4284, -49.2733
	jobs := &fakeScrapeJobs{}
	geocoder := &fakeGeocoder{}
	ceps := NewCEPService(&fakeCEPClient{location: domain.CEPLocation{City: "Curitiba", Latitude: &latitude, Longitude: &longitude}})
	service := NewProspectorService(jobs, geocoder).WithCEPResolver(ceps)

	created, err := service.Create(context.Background(), domain.ProspectorParams{
		Keyword: "padarias",
		CEP:     "80010-000",
		City:    "Outra",
		Address: &domain.Address{Street: "Rua X"},
	})

	require.NoError(t, err)
	require.Equal(t, "Curitiba", created.City)
	require.Zero(t, geocoder.totalCallCount)
	require.Len(t, jobs.created, 1)
	require.Equal(t, "padarias em Curitiba", jobs.created[0].Keyword)
	require.Equal(t, "-25.4284,-49.2733", jobs.created[0].GeoCoordinates)
}

func TestProspectorCreateWithCEPNotFound(t *testing.T) {
	jobs := &fakeScrapeJobs{}
	ceps := NewCEPService(&fakeCEPClient{err: domain.ErrCEPNotFound})
	service := NewProspectorService(jobs, &fakeGeocoder{}).WithCEPResolver(ceps)

	_, err := service.Create(context.Background(), domain.ProspectorParams{Keyword: "padarias", CEP: "80010000"})

	requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeCEPNotFound)
	require.Empty(t, jobs.created)
}

func TestProspectorCreateWithCEPWithoutCoordinates(t *testing.T) {
	ceps := NewCEPService(&fakeCEPClient{location: domain.CEPLocation{City: "Curitiba"}})
	service := NewProspectorService(&fakeScrapeJobs{}, &fakeGeocoder{}).WithCEPResolver(ceps)

	_, err := service.Create(context.Background(), domain.ProspectorParams{Keyword: "padarias", CEP: "80010000"})

	requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeCEPNotFound)
}

func TestProspectorCreateWithCEPUnavailable(t *testing.T) {
	jobs := &fakeScrapeJobs{}
	ceps := NewCEPService(&fakeCEPClient{err: domain.ErrGeocoderUnavailable})
	service := NewProspectorService(jobs, &fakeGeocoder{}).WithCEPResolver(ceps)

	_, err := service.Create(context.Background(), domain.ProspectorParams{Keyword: "padarias", CEP: "80010000"})

	requireAppErrorCode(t, err, http.StatusBadGateway, domain.CodeGeocoderUnavailable)
	require.Empty(t, jobs.created)
}

func TestProspectorCreateWithInvalidCEP(t *testing.T) {
	ceps := NewCEPService(&fakeCEPClient{})
	service := NewProspectorService(&fakeScrapeJobs{}, &fakeGeocoder{}).WithCEPResolver(ceps)

	_, err := service.Create(context.Background(), domain.ProspectorParams{Keyword: "padarias", CEP: "123"})

	requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeInvalidInput)
}

func TestProspectorCreateWithCEPWithoutCity(t *testing.T) {
	latitude, longitude := -25.4284, -49.2733
	jobs := &fakeScrapeJobs{}
	ceps := NewCEPService(&fakeCEPClient{location: domain.CEPLocation{Latitude: &latitude, Longitude: &longitude}})
	service := NewProspectorService(jobs, &fakeGeocoder{}).WithCEPResolver(ceps)

	_, err := service.Create(context.Background(), domain.ProspectorParams{Keyword: "padarias", CEP: "80010000", City: "Outra"})

	requireAppErrorCode(t, err, http.StatusBadRequest, domain.CodeCEPNotFound)
	require.Empty(t, jobs.created)
}

func TestProspectorCreateWithCEPWithoutResolver(t *testing.T) {
	jobs := &fakeScrapeJobs{}
	service := NewProspectorService(jobs, &fakeGeocoder{})

	_, err := service.Create(context.Background(), domain.ProspectorParams{Keyword: "padarias", CEP: "80010000"})

	requireAppErrorCode(t, err, http.StatusServiceUnavailable, domain.CodeServiceNotConfigured)
	require.Empty(t, jobs.created)
}
