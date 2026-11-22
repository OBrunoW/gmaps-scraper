package geocoder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

type capturedRequest struct {
	path   string
	query  url.Values
	header http.Header
}

func newTestServer(t *testing.T, status int, body string) (*Nominatim, *[]capturedRequest) {
	t.Helper()

	captured := &[]capturedRequest{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = append(*captured, capturedRequest{path: r.URL.Path, query: r.URL.Query(), header: r.Header.Clone()})
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	geocoder := NewNominatim(server.URL + "/")
	geocoder.limiter.interval = 0

	return geocoder, captured
}

func TestGeocodeCitySuccess(t *testing.T) {
	geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"-25.4284","lon":"-49.2733"}]`)

	point, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "PR")

	require.NoError(t, err)
	require.Equal(t, domain.GeoPoint{Lat: -25.4284, Lng: -49.2733}, point)
	require.Len(t, *captured, 1)

	request := (*captured)[0]
	require.Equal(t, "/search", request.path)
	require.Equal(t, "Curitiba", request.query.Get("city"))
	require.Equal(t, "Paraná", request.query.Get("state"))
	require.Equal(t, "br", request.query.Get("countrycodes"))
	require.Equal(t, "jsonv2", request.query.Get("format"))
	require.Equal(t, "1", request.query.Get("limit"))
	require.False(t, request.query.Has("street"))
}

func TestGeocodeSendsHeaders(t *testing.T) {
	geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"1","lon":"2"}]`)

	_, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "PR")

	require.NoError(t, err)

	header := (*captured)[0].header
	require.Equal(t, "gmaps-scraper", header.Get("User-Agent"))
	require.Equal(t, "pt-BR", header.Get("Accept-Language"))
	require.Equal(t, "application/json", header.Get("Accept"))
}

func TestGeocodeCityWithoutUFOmitsState(t *testing.T) {
	geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"1","lon":"2"}]`)

	_, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "")

	require.NoError(t, err)
	require.False(t, (*captured)[0].query.Has("state"))
}

func TestGeocodeUnknownUFIsSentAsReceived(t *testing.T) {
	geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"1","lon":"2"}]`)

	_, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "XX")

	require.NoError(t, err)
	require.Equal(t, "XX", (*captured)[0].query.Get("state"))
}

func TestGeocodeMapsAllUFsToStateNames(t *testing.T) {
	require.Len(t, stateNamesByUF, 27)

	geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"1","lon":"2"}]`)

	for uf, name := range stateNamesByUF {
		_, err := geocoder.GeocodeCity(context.Background(), "Cidade", uf)
		require.NoError(t, err)
		require.Equal(t, name, (*captured)[len(*captured)-1].query.Get("state"))
	}
}

func TestGeocodeAddressBuildsStructuredStreet(t *testing.T) {
	cases := []struct {
		name    string
		address domain.Address
		want    string
	}{
		{"with number", domain.Address{Street: "Rua XV de Novembro", Number: "100", Neighborhood: "Centro"}, "100 Rua XV de Novembro"},
		{"without number", domain.Address{Street: "Rua XV de Novembro"}, "Rua XV de Novembro"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"-25.4","lon":"-49.2"}]`)

			point, err := geocoder.GeocodeAddress(context.Background(), tc.address, "Curitiba", "PR")

			require.NoError(t, err)
			require.Equal(t, domain.GeoPoint{Lat: -25.4, Lng: -49.2}, point)

			query := (*captured)[0].query
			require.Equal(t, tc.want, query.Get("street"))
			require.Equal(t, "Curitiba", query.Get("city"))
			require.Equal(t, "Paraná", query.Get("state"))
			require.False(t, query.Has("neighborhood"))
		})
	}
}

func TestGeocodeEmptyListIsLocationNotFound(t *testing.T) {
	geocoder, _ := newTestServer(t, http.StatusOK, `[]`)

	_, err := geocoder.GeocodeCity(context.Background(), "Nowhere", "PR")

	require.ErrorIs(t, err, domain.ErrLocationNotFound)
	require.NotErrorIs(t, err, domain.ErrGeocoderUnavailable)
}

func TestGeocodeUnavailableCases(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"too many requests", http.StatusTooManyRequests, `[]`},
		{"server error", http.StatusInternalServerError, `[]`},
		{"bad request", http.StatusBadRequest, `[]`},
		{"invalid json", http.StatusOK, `not json`},
		{"object instead of list", http.StatusOK, `{"lat":"1"}`},
		{"non numeric latitude", http.StatusOK, `[{"lat":"abc","lon":"2"}]`},
		{"non numeric longitude", http.StatusOK, `[{"lat":"1","lon":"abc"}]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			geocoder, _ := newTestServer(t, tc.status, tc.body)

			_, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "PR")

			require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
			require.NotErrorIs(t, err, domain.ErrLocationNotFound)
		})
	}
}

func TestGeocodeNetworkFailureIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	baseURL := server.URL
	server.Close()

	geocoder := NewNominatim(baseURL)
	geocoder.limiter.interval = 0

	_, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "PR")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
}

func TestGeocodeCancelledContextIsUnavailable(t *testing.T) {
	geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"1","lon":"2"}]`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := geocoder.GeocodeCity(ctx, "Curitiba", "PR")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, *captured)
}

func TestGeocodeRateLimiterSpacesRequests(t *testing.T) {
	geocoder, _ := newTestServer(t, http.StatusOK, `[{"lat":"1","lon":"2"}]`)
	geocoder.limiter.interval = 80 * time.Millisecond

	started := time.Now()

	for range 3 {
		_, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "PR")
		require.NoError(t, err)
	}

	require.GreaterOrEqual(t, time.Since(started), 160*time.Millisecond)
}

func TestGeocodeRateLimiterRespectsContext(t *testing.T) {
	geocoder, captured := newTestServer(t, http.StatusOK, `[{"lat":"1","lon":"2"}]`)
	geocoder.limiter.interval = time.Hour

	_, err := geocoder.GeocodeCity(context.Background(), "Curitiba", "PR")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err = geocoder.GeocodeCity(ctx, "Curitiba", "PR")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Len(t, *captured, 1)
}

func TestNewNominatimTrimsTrailingSlash(t *testing.T) {
	require.Equal(t, "https://example.test", NewNominatim("https://example.test//").baseURL)
}
