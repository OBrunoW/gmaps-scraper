package domain

import (
	"errors"
	"testing"
	"time"
)

func validParams() ScrapeParams {
	return ScrapeParams{Keyword: "pizza", TimeoutSecs: 300}
}

func TestScrapeParamsValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ScrapeParams)
		wantErr string
	}{
		{"valid", func(p *ScrapeParams) {}, ""},
		{"missing keyword", func(p *ScrapeParams) { p.Keyword = "" }, "keyword is required"},
		{"two commas", func(p *ScrapeParams) { p.GeoCoordinates = "1,2,3" }, "geo_coordinates must contain exactly one comma"},
		{"no comma", func(p *ScrapeParams) { p.GeoCoordinates = "12" }, "geo_coordinates must contain exactly one comma"},
		{"bad latitude", func(p *ScrapeParams) { p.GeoCoordinates = "abc,2" }, `invalid latitude: strconv.ParseFloat: parsing "abc": invalid syntax`},
		{"bad longitude", func(p *ScrapeParams) { p.GeoCoordinates = "1,xyz" }, `invalid longitude: strconv.ParseFloat: parsing "xyz": invalid syntax`},
		{"latitude range", func(p *ScrapeParams) { p.GeoCoordinates = "91,2" }, "latitude must be between -90 and 90"},
		{"longitude range", func(p *ScrapeParams) { p.GeoCoordinates = "1,181" }, "longitude must be between -180 and 180"},
		{"valid geo with spaces", func(p *ScrapeParams) { p.GeoCoordinates = " 40.7 , -74.0 " }, ""},
		{"fast mode without geo", func(p *ScrapeParams) { p.FastMode = true }, "geo_coordinates are required in fast mode"},
		{"fast mode without zoom", func(p *ScrapeParams) { p.FastMode = true; p.GeoCoordinates = "1,2" }, "zoom is required in fast mode"},
		{"fast mode valid", func(p *ScrapeParams) { p.FastMode = true; p.GeoCoordinates = "1,2"; p.Zoom = 10 }, ""},
		{"zoom too high", func(p *ScrapeParams) { p.Zoom = 22 }, "zoom must be between 1 and 21"},
		{"zoom negative", func(p *ScrapeParams) { p.Zoom = -1 }, "zoom must be between 1 and 21"},
		{"negative radius", func(p *ScrapeParams) { p.Radius = -1 }, "radius must be non-negative"},
		{"max depth too high", func(p *ScrapeParams) { p.MaxDepth = 101 }, "max_depth must be between 0 and 100"},
		{"max depth negative", func(p *ScrapeParams) { p.MaxDepth = -1 }, "max_depth must be between 0 and 100"},
		{"timeout zero", func(p *ScrapeParams) { p.TimeoutSecs = 0 }, "timeout must be between 1 and 300 seconds"},
		{"timeout too high", func(p *ScrapeParams) { p.TimeoutSecs = 301 }, "timeout must be between 1 and 300 seconds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := validParams()
			tt.mutate(&params)

			err := params.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			var appErr *AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("error = %v, want AppError", err)
			}
			if appErr.Code != CodeInvalidInput {
				t.Errorf("code = %q, want %q", appErr.Code, CodeInvalidInput)
			}
			if appErr.Message != tt.wantErr {
				t.Errorf("message = %q, want %q", appErr.Message, tt.wantErr)
			}
		})
	}
}

func TestScrapeParamsSetDefaults(t *testing.T) {
	params := ScrapeParams{Keyword: "pizza"}
	params.SetDefaults()

	if params.Lang != "en" {
		t.Errorf("lang = %q, want en", params.Lang)
	}
	if params.MaxDepth != 1 {
		t.Errorf("max depth = %d, want 1", params.MaxDepth)
	}
	if params.TimeoutSecs != DefaultScrapeTimeoutSecs {
		t.Errorf("timeout = %d, want %d", params.TimeoutSecs, DefaultScrapeTimeoutSecs)
	}
}

func TestScrapeParamsSetDefaultsKeepsValues(t *testing.T) {
	params := ScrapeParams{Lang: "pt", MaxDepth: 5, TimeoutSecs: 60}
	params.SetDefaults()

	if params.Lang != "pt" || params.MaxDepth != 5 || params.TimeoutSecs != 60 {
		t.Errorf("defaults overwrote values: %+v", params)
	}
}

func TestEffectiveScrapeTimeout(t *testing.T) {
	tests := []struct {
		secs int
		want time.Duration
	}{
		{-5, MaxScrapeTimeout},
		{0, MaxScrapeTimeout},
		{1, time.Second},
		{120, 2 * time.Minute},
		{300, MaxScrapeTimeout},
		{301, MaxScrapeTimeout},
		{100000, MaxScrapeTimeout},
	}

	for _, tt := range tests {
		if got := EffectiveScrapeTimeout(tt.secs); got != tt.want {
			t.Errorf("EffectiveScrapeTimeout(%d) = %v, want %v", tt.secs, got, tt.want)
		}
	}
}
