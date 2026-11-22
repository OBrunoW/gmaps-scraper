package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
)

type rawPlace struct {
	PlaceID         string `json:"place_id"`
	Cid             string `json:"cid"`
	Link            string `json:"link"`
	Title           string `json:"title"`
	Phone           string `json:"phone"`
	WebSite         string `json:"web_site"`
	Address         string `json:"address"`
	Category        string `json:"category"`
	CompleteAddress struct {
		City  string `json:"city"`
		State string `json:"state"`
	} `json:"complete_address"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Longtitude float64 `json:"longtitude"`
}

func NormalizeLeads(results json.RawMessage) ([]domain.Lead, error) {
	leads := []domain.Lead{}

	trimmed := bytes.TrimSpace(results)
	if len(trimmed) == 0 {
		return leads, nil
	}

	if !json.Valid(trimmed) {
		return nil, fmt.Errorf("invalid results json")
	}

	if trimmed[0] != '[' {
		return leads, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, fmt.Errorf("decode results: %w", err)
	}

	seen := make(map[string]struct{}, len(items))

	for _, item := range items {
		var place rawPlace
		if err := json.Unmarshal(item, &place); err != nil {
			continue
		}

		lead, ok := place.toLead()
		if !ok {
			continue
		}

		if _, duplicated := seen[lead.Key]; duplicated {
			continue
		}

		seen[lead.Key] = struct{}{}
		leads = append(leads, lead)
	}

	return leads, nil
}

func (p rawPlace) toLead() (domain.Lead, bool) {
	title := strings.TrimSpace(p.Title)
	key := p.key()

	if title == "" || key == "" {
		return domain.Lead{}, false
	}

	lead := domain.Lead{
		Key:      key,
		Title:    title,
		Phone:    textOrNil(p.Phone),
		Website:  textOrNil(p.WebSite),
		Address:  textOrNil(p.Address),
		City:     textOrNil(p.CompleteAddress.City),
		State:    textOrNil(p.CompleteAddress.State),
		Category: textOrNil(p.Category),
		MapsURL:  textOrNil(p.Link),
	}

	lead.Lat, lead.Lng = p.coordinates()

	return lead, true
}

func (p rawPlace) key() string {
	if placeID := strings.TrimSpace(p.PlaceID); placeID != "" {
		return "place:" + placeID
	}

	if cid := strings.TrimSpace(p.Cid); cid != "" {
		return "cid:" + cid
	}

	if link := strings.TrimSpace(p.Link); link != "" {
		return "link:" + link
	}

	return ""
}

func (p rawPlace) coordinates() (*float64, *float64) {
	longitude := p.Longitude
	if longitude == 0 {
		longitude = p.Longtitude
	}

	if p.Latitude == 0 && longitude == 0 {
		return nil, nil
	}

	latitude := p.Latitude

	return &latitude, &longitude
}

func textOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
