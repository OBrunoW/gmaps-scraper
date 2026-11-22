package application

import (
	"context"
	"errors"
	"strings"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const cepDigits = 8

type cepLookupClient interface {
	LookupCEP(ctx context.Context, cep string) (domain.CEPLocation, error)
}

type CEPService struct {
	client cepLookupClient
}

func NewCEPService(client cepLookupClient) *CEPService {
	return &CEPService{client: client}
}

func (s *CEPService) Lookup(ctx context.Context, cep string) (domain.CEPLocation, error) {
	digits := onlyDigits(cep)
	if len(digits) != cepDigits {
		return domain.CEPLocation{}, domain.InvalidInput("cep deve ter 8 digitos")
	}

	location, err := s.client.LookupCEP(ctx, digits)
	if err == nil {
		return location, nil
	}

	if errors.Is(err, domain.ErrCEPNotFound) {
		return domain.CEPLocation{}, domain.CEPNotFound()
	}

	log.Error("cep lookup unavailable", "error", err)

	return domain.CEPLocation{}, domain.GeocoderUnavailable(err)
}

func onlyDigits(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}

	return builder.String()
}
