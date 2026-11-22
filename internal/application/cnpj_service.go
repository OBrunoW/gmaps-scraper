package application

import (
	"context"
	"errors"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const cnpjDigits = 14

var (
	cnpjFirstWeights  = []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	cnpjSecondWeights = []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
)

type cnpjLookupClient interface {
	LookupCNPJ(ctx context.Context, cnpj string) (domain.CNPJCompany, error)
}

type CNPJService struct {
	client cnpjLookupClient
}

func NewCNPJService(client cnpjLookupClient) *CNPJService {
	return &CNPJService{client: client}
}

func (s *CNPJService) Lookup(ctx context.Context, cnpj string) (domain.CNPJCompany, error) {
	digits := onlyDigits(cnpj)
	if !validCNPJ(digits) {
		return domain.CNPJCompany{}, domain.InvalidCNPJ()
	}

	company, err := s.client.LookupCNPJ(ctx, digits)
	if err == nil {
		return company, nil
	}

	if errors.Is(err, domain.ErrInvalidCNPJ) {
		return domain.CNPJCompany{}, domain.InvalidCNPJ()
	}

	log.Error("cnpj lookup unavailable", "error", err)

	return domain.CNPJCompany{}, domain.UpstreamUnavailable(err)
}

func validCNPJ(digits string) bool {
	if len(digits) != cnpjDigits || allSameDigit(digits) {
		return false
	}

	first := cnpjCheckDigit(digits[:12], cnpjFirstWeights)
	second := cnpjCheckDigit(digits[:12]+string(rune('0'+first)), cnpjSecondWeights)

	return int(digits[12]-'0') == first && int(digits[13]-'0') == second
}

func cnpjCheckDigit(base string, weights []int) int {
	sum := 0
	for index, weight := range weights {
		sum += int(base[index]-'0') * weight
	}

	remainder := sum % 11
	if remainder < 2 {
		return 0
	}

	return 11 - remainder
}

func allSameDigit(digits string) bool {
	for index := 1; index < len(digits); index++ {
		if digits[index] != digits[0] {
			return false
		}
	}

	return true
}
