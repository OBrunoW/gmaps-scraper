package domain

import (
	"strconv"
	"strings"
	"time"
)

const (
	MaxScrapeTimeout         = 5 * time.Minute
	DefaultScrapeTimeoutSecs = 300
)

func EffectiveScrapeTimeout(timeoutSecs int) time.Duration {
	if timeoutSecs <= 0 {
		return MaxScrapeTimeout
	}

	timeout := time.Duration(timeoutSecs) * time.Second
	if timeout > MaxScrapeTimeout {
		return MaxScrapeTimeout
	}

	return timeout
}

type ScrapeParams struct {
	Keyword        string
	Lang           string
	MaxDepth       int
	Email          bool
	GeoCoordinates string
	Zoom           int
	Radius         float64
	FastMode       bool
	ExtraReviews   bool
	TimeoutSecs    int
}

func (p *ScrapeParams) SetDefaults() {
	if p.Lang == "" {
		p.Lang = "en"
	}

	if p.MaxDepth == 0 {
		p.MaxDepth = 1
	}

	if p.TimeoutSecs == 0 {
		p.TimeoutSecs = DefaultScrapeTimeoutSecs
	}
}

func (p ScrapeParams) Validate() error {
	if p.Keyword == "" {
		return InvalidInput("keyword is required")
	}

	if p.GeoCoordinates != "" {
		if err := validateGeoCoordinates(p.GeoCoordinates); err != nil {
			return err
		}
	}

	if p.FastMode {
		if p.GeoCoordinates == "" {
			return InvalidInput("geo_coordinates are required in fast mode")
		}

		if p.Zoom == 0 {
			return InvalidInput("zoom is required in fast mode")
		}
	}

	if p.Zoom != 0 && (p.Zoom < 1 || p.Zoom > 21) {
		return InvalidInput("zoom must be between 1 and 21")
	}

	if p.Radius < 0 {
		return InvalidInput("radius must be non-negative")
	}

	if p.MaxDepth < 0 || p.MaxDepth > 100 {
		return InvalidInput("max_depth must be between 0 and 100")
	}

	if p.TimeoutSecs < 1 || p.TimeoutSecs > 300 {
		return InvalidInput("timeout must be between 1 and 300 seconds")
	}

	return nil
}

func validateGeoCoordinates(coordinates string) error {
	if strings.Count(coordinates, ",") != 1 {
		return InvalidInput("geo_coordinates must contain exactly one comma")
	}

	latitudeText, longitudeText, ok := strings.Cut(coordinates, ",")
	if !ok {
		return InvalidInput("geo_coordinates must be in format 'lat,lon'")
	}

	latitude, err := strconv.ParseFloat(strings.TrimSpace(latitudeText), 64)
	if err != nil {
		return InvalidInput("invalid latitude: " + err.Error())
	}

	longitude, err := strconv.ParseFloat(strings.TrimSpace(longitudeText), 64)
	if err != nil {
		return InvalidInput("invalid longitude: " + err.Error())
	}

	if latitude < -90 || latitude > 90 {
		return InvalidInput("latitude must be between -90 and 90")
	}

	if longitude < -180 || longitude > 180 {
		return InvalidInput("longitude must be between -180 and 180")
	}

	return nil
}
