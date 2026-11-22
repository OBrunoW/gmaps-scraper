package domain

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	DefaultRadiusKm = 5
	MinRadiusKm     = 1
	MaxRadiusKm     = 50

	maxStreetRunes       = 120
	maxNumberRunes       = 20
	maxNeighborhoodRunes = 80
)

var ufPattern = regexp.MustCompile(`^[A-Z]{2}$`)

type Address struct {
	Street       string
	Number       string
	Neighborhood string
}

type GeoPoint struct {
	Lat float64
	Lng float64
}

type ProspectorParams struct {
	Keyword  string
	City     string
	UF       string
	Address  *Address
	CEP      string
	RadiusKm *float64
}

func (p *ProspectorParams) SetDefaults() {
	p.Keyword = strings.TrimSpace(p.Keyword)
	p.City = strings.TrimSpace(p.City)
	p.UF = strings.ToUpper(strings.TrimSpace(p.UF))
	p.Address = normalizeAddress(p.Address)
	p.CEP = strings.TrimSpace(p.CEP)
	p.RadiusKm = normalizeRadius(p.RadiusKm)
}

func (p ProspectorParams) Validate() error {
	if !runeLengthBetween(p.Keyword, 2, 120) {
		return InvalidInput("keyword deve ter entre 2 e 120 caracteres")
	}

	if p.CEP != "" {
		return nil
	}

	if !runeLengthBetween(p.City, 2, 80) {
		return InvalidInput("city deve ter entre 2 e 80 caracteres")
	}

	if p.UF != "" && !ufPattern.MatchString(p.UF) {
		return InvalidInput("uf deve ter 2 letras")
	}

	if p.Address != nil && p.UF == "" {
		return InvalidInput("uf e obrigatoria quando address e informado")
	}

	return nil
}

type Lead struct {
	Key      string
	Title    string
	Phone    *string
	Website  *string
	Address  *string
	City     *string
	State    *string
	Lat      *float64
	Lng      *float64
	Category *string
	MapsURL  *string
}

type ProspectorJob struct {
	PublicID    string
	Status      JobStatus
	Keyword     string
	ResultCount int
	Error       string
	Leads       []Lead
}

func normalizeAddress(address *Address) *Address {
	if address == nil {
		return nil
	}

	street := truncateRunes(strings.TrimSpace(address.Street), maxStreetRunes)
	if street == "" {
		return nil
	}

	return &Address{
		Street:       street,
		Number:       truncateRunes(strings.TrimSpace(address.Number), maxNumberRunes),
		Neighborhood: truncateRunes(strings.TrimSpace(address.Neighborhood), maxNeighborhoodRunes),
	}
}

func normalizeRadius(radius *float64) *float64 {
	value := float64(DefaultRadiusKm)
	if radius != nil {
		value = math.Round(*radius)
	}

	value = math.Max(MinRadiusKm, math.Min(MaxRadiusKm, value))

	return &value
}

func runeLengthBetween(value string, minRunes, maxRunes int) bool {
	count := utf8.RuneCountInString(value)
	return count >= minRunes && count <= maxRunes
}

func truncateRunes(value string, maxRunes int) string {
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}

	return string([]rune(value)[:maxRunes])
}
