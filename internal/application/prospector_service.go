package application

import (
	"context"
	"errors"
	"strconv"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const (
	prospectorLang        = "pt"
	prospectorZoom        = 14
	prospectorMaxDepth    = 1
	prospectorTimeoutSecs = 300
	metersPerKilometer    = 1000
)

type scrapeJobs interface {
	Create(ctx context.Context, params domain.ScrapeParams) (CreatedJob, error)
	Get(ctx context.Context, publicID string) (JobView, error)
}

type CreatedProspectorJob struct {
	PublicID string
	Status   domain.JobStatus
	Keyword  string
	City     string
	RadiusKm int
}

type cepResolver interface {
	Lookup(ctx context.Context, cep string) (domain.CEPLocation, error)
}

type ProspectorService struct {
	jobs     scrapeJobs
	geocoder domain.Geocoder
	ceps     cepResolver
}

func NewProspectorService(jobs scrapeJobs, geocoder domain.Geocoder) *ProspectorService {
	return &ProspectorService{jobs: jobs, geocoder: geocoder}
}

func (s *ProspectorService) WithCEPResolver(resolver cepResolver) *ProspectorService {
	s.ceps = resolver
	return s
}

func (s *ProspectorService) Create(ctx context.Context, params domain.ProspectorParams) (CreatedProspectorJob, error) {
	params.SetDefaults()

	if err := params.Validate(); err != nil {
		return CreatedProspectorJob{}, err
	}

	center, err := s.resolveCenter(ctx, &params)
	if err != nil {
		return CreatedProspectorJob{}, err
	}

	radiusKm := int(*params.RadiusKm)

	created, err := s.jobs.Create(ctx, domain.ScrapeParams{
		Keyword:        params.Keyword + " em " + params.City,
		Lang:           prospectorLang,
		MaxDepth:       prospectorMaxDepth,
		GeoCoordinates: formatCoordinates(center),
		Zoom:           prospectorZoom,
		Radius:         float64(radiusKm * metersPerKilometer),
		FastMode:       true,
		TimeoutSecs:    prospectorTimeoutSecs,
	})
	if err != nil {
		return CreatedProspectorJob{}, err
	}

	return CreatedProspectorJob{
		PublicID: created.PublicID,
		Status:   created.Status,
		Keyword:  params.Keyword,
		City:     params.City,
		RadiusKm: radiusKm,
	}, nil
}

func (s *ProspectorService) Get(ctx context.Context, publicID string) (domain.ProspectorJob, error) {
	view, err := s.jobs.Get(ctx, publicID)
	if err != nil {
		return domain.ProspectorJob{}, err
	}

	job := domain.ProspectorJob{
		PublicID:    view.PublicID,
		Status:      view.Status,
		Keyword:     view.Keyword,
		ResultCount: view.ResultCount,
		Error:       view.Error,
	}

	if view.Status != domain.StatusCompleted {
		return job, nil
	}

	leads, err := NormalizeLeads(view.Results)
	if err != nil {
		log.Error("failed to normalize job results", "error", err, "job_id", publicID)
		return domain.ProspectorJob{}, domain.Internal("failed to read job results", err)
	}

	job.Leads = leads

	return job, nil
}

func (s *ProspectorService) resolveCenter(ctx context.Context, params *domain.ProspectorParams) (domain.GeoPoint, error) {
	if params.CEP == "" {
		return s.geocode(ctx, *params)
	}

	if s.ceps == nil {
		return domain.GeoPoint{}, domain.ServiceNotConfigured()
	}

	location, err := s.ceps.Lookup(ctx, params.CEP)
	if err != nil {
		return domain.GeoPoint{}, err
	}

	if location.Latitude == nil || location.Longitude == nil {
		return domain.GeoPoint{}, domain.CEPNotFound()
	}

	if location.City == "" {
		return domain.GeoPoint{}, domain.CEPWithoutCity()
	}

	params.City = location.City

	return domain.GeoPoint{Lat: *location.Latitude, Lng: *location.Longitude}, nil
}

func (s *ProspectorService) geocode(ctx context.Context, params domain.ProspectorParams) (domain.GeoPoint, error) {
	if params.Address != nil {
		point, err := s.geocoder.GeocodeAddress(ctx, *params.Address, params.City, params.UF)
		return point, geocodeError(err, domain.AddressNotFound())
	}

	point, err := s.geocoder.GeocodeCity(ctx, params.City, params.UF)

	return point, geocodeError(err, domain.CityNotFound())
}

func geocodeError(err error, notFound *domain.AppError) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, domain.ErrLocationNotFound) {
		return notFound
	}

	log.Error("geocoder unavailable", "error", err)

	return domain.GeocoderUnavailable(err)
}

func formatCoordinates(point domain.GeoPoint) string {
	return strconv.FormatFloat(point.Lat, 'f', -1, 64) + "," + strconv.FormatFloat(point.Lng, 'f', -1, 64)
}
