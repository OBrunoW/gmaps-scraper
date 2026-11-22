package geocoder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const (
	requestTimeout  = 10 * time.Second
	minimumInterval = time.Second
	maxResponseSize = 1 << 20
	userAgent       = "gmaps-scraper"
	acceptLanguage  = "pt-BR"
)

var stateNamesByUF = map[string]string{
	"AC": "Acre",
	"AL": "Alagoas",
	"AP": "Amapá",
	"AM": "Amazonas",
	"BA": "Bahia",
	"CE": "Ceará",
	"DF": "Distrito Federal",
	"ES": "Espírito Santo",
	"GO": "Goiás",
	"MA": "Maranhão",
	"MT": "Mato Grosso",
	"MS": "Mato Grosso do Sul",
	"MG": "Minas Gerais",
	"PA": "Pará",
	"PB": "Paraíba",
	"PR": "Paraná",
	"PE": "Pernambuco",
	"PI": "Piauí",
	"RJ": "Rio de Janeiro",
	"RN": "Rio Grande do Norte",
	"RS": "Rio Grande do Sul",
	"RO": "Rondônia",
	"RR": "Roraima",
	"SC": "Santa Catarina",
	"SP": "São Paulo",
	"SE": "Sergipe",
	"TO": "Tocantins",
}

type Nominatim struct {
	baseURL string
	client  *http.Client
	limiter *rateLimiter
}

func NewNominatim(baseURL string) *Nominatim {
	return &Nominatim{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: requestTimeout},
		limiter: &rateLimiter{interval: minimumInterval},
	}
}

func (n *Nominatim) GeocodeCity(ctx context.Context, city, uf string) (domain.GeoPoint, error) {
	query := url.Values{}
	query.Set("city", city)
	setState(query, uf)

	return n.search(ctx, query)
}

func (n *Nominatim) GeocodeAddress(ctx context.Context, address domain.Address, city, uf string) (domain.GeoPoint, error) {
	query := url.Values{}
	query.Set("street", strings.TrimSpace(address.Number+" "+address.Street))
	query.Set("city", city)
	setState(query, uf)

	return n.search(ctx, query)
}

type searchResult struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

func (n *Nominatim) search(ctx context.Context, query url.Values) (domain.GeoPoint, error) {
	query.Set("countrycodes", "br")
	query.Set("format", "jsonv2")
	query.Set("limit", "1")

	body, err := n.fetch(ctx, n.baseURL+"/search?"+query.Encode())
	if err != nil {
		return domain.GeoPoint{}, err
	}

	var results []searchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return domain.GeoPoint{}, unavailable("decode response", err)
	}

	if len(results) == 0 {
		return domain.GeoPoint{}, domain.ErrLocationNotFound
	}

	return parsePoint(results[0])
}

func (n *Nominatim) fetch(ctx context.Context, target string) ([]byte, error) {
	if err := n.limiter.wait(ctx); err != nil {
		return nil, unavailable("wait for rate limit", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, unavailable("build request", err)
	}

	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", acceptLanguage)

	response, err := n.client.Do(request)
	if err != nil {
		return nil, unavailable("send request", stripURL(err))
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		log.Error("geocoder returned non success status", "status", response.StatusCode)
		return nil, unavailable("unexpected status", fmt.Errorf("status %d", response.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize))
	if err != nil {
		return nil, unavailable("read response", stripURL(err))
	}

	return body, nil
}

func parsePoint(result searchResult) (domain.GeoPoint, error) {
	lat, err := strconv.ParseFloat(result.Lat, 64)
	if err != nil {
		return domain.GeoPoint{}, unavailable("parse latitude", err)
	}

	lng, err := strconv.ParseFloat(result.Lon, 64)
	if err != nil {
		return domain.GeoPoint{}, unavailable("parse longitude", err)
	}

	return domain.GeoPoint{Lat: lat, Lng: lng}, nil
}

func setState(query url.Values, uf string) {
	uf = strings.TrimSpace(uf)
	if uf == "" {
		return
	}

	if name, ok := stateNamesByUF[strings.ToUpper(uf)]; ok {
		query.Set("state", name)
		return
	}

	query.Set("state", uf)
}

func unavailable(step string, cause error) error {
	return fmt.Errorf("%s: %w: %w", step, domain.ErrGeocoderUnavailable, cause)
}

func stripURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}

	return err
}

type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func (l *rateLimiter) wait(ctx context.Context) error {
	l.mu.Lock()

	now := time.Now()
	slot := l.next
	if slot.Before(now) {
		slot = now
	}

	l.next = slot.Add(l.interval)
	l.mu.Unlock()

	delay := slot.Sub(now)
	if delay <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
