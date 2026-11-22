package domain

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func floatPointer(value float64) *float64 {
	return &value
}

func TestProspectorParamsSetDefaultsNormalizesText(t *testing.T) {
	params := ProspectorParams{
		Keyword: "  padarias ",
		City:    " Curitiba  ",
		UF:      " pr ",
		Address: &Address{Street: "  Rua XV ", Number: " 100 ", Neighborhood: " Centro "},
	}

	params.SetDefaults()

	require.Equal(t, "padarias", params.Keyword)
	require.Equal(t, "Curitiba", params.City)
	require.Equal(t, "PR", params.UF)
	require.Equal(t, &Address{Street: "Rua XV", Number: "100", Neighborhood: "Centro"}, params.Address)
}

func TestProspectorParamsSetDefaultsDropsAddressWithoutStreet(t *testing.T) {
	params := ProspectorParams{Address: &Address{Street: "   ", Number: "10"}}

	params.SetDefaults()

	require.Nil(t, params.Address)
}

func TestProspectorParamsSetDefaultsTruncatesByRunes(t *testing.T) {
	params := ProspectorParams{Address: &Address{
		Street:       strings.Repeat("ã", 130),
		Number:       strings.Repeat("9", 30),
		Neighborhood: strings.Repeat("é", 90),
	}}

	params.SetDefaults()

	require.Equal(t, strings.Repeat("ã", 120), params.Address.Street)
	require.Equal(t, strings.Repeat("9", 20), params.Address.Number)
	require.Equal(t, strings.Repeat("é", 80), params.Address.Neighborhood)
}

func TestProspectorParamsSetDefaultsRadius(t *testing.T) {
	cases := []struct {
		name  string
		input *float64
		want  float64
	}{
		{"absent", nil, 5},
		{"zero", floatPointer(0), 1},
		{"rounds down", floatPointer(0.4), 1},
		{"rounds up", floatPointer(7.6), 8},
		{"negative", floatPointer(-3), 1},
		{"upper bound", floatPointer(50), 50},
		{"clamped", floatPointer(80), 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := ProspectorParams{RadiusKm: tc.input}

			params.SetDefaults()

			require.Equal(t, tc.want, *params.RadiusKm)
		})
	}
}

func TestProspectorParamsValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProspectorParams)
		want   string
	}{
		{"valid", func(p *ProspectorParams) {}, ""},
		{"valid without uf", func(p *ProspectorParams) { p.UF = "" }, ""},
		{"keyword one rune", func(p *ProspectorParams) { p.Keyword = "a" }, "keyword deve ter entre 2 e 120 caracteres"},
		{"keyword two runes", func(p *ProspectorParams) { p.Keyword = "ab" }, ""},
		{"keyword 120 runes", func(p *ProspectorParams) { p.Keyword = strings.Repeat("ã", 120) }, ""},
		{"keyword 121 runes", func(p *ProspectorParams) { p.Keyword = strings.Repeat("ã", 121) }, "keyword deve ter entre 2 e 120 caracteres"},
		{"city one rune", func(p *ProspectorParams) { p.City = "x" }, "city deve ter entre 2 e 80 caracteres"},
		{"city 80 runes", func(p *ProspectorParams) { p.City = strings.Repeat("ã", 80) }, ""},
		{"city 81 runes", func(p *ProspectorParams) { p.City = strings.Repeat("ã", 81) }, "city deve ter entre 2 e 80 caracteres"},
		{"uf three letters", func(p *ProspectorParams) { p.UF = "PRR" }, "uf deve ter 2 letras"},
		{"uf with digit", func(p *ProspectorParams) { p.UF = "P1" }, "uf deve ter 2 letras"},
		{"uf lowercase", func(p *ProspectorParams) { p.UF = "pr" }, "uf deve ter 2 letras"},
		{"address without uf", func(p *ProspectorParams) {
			p.UF = ""
			p.Address = &Address{Street: "Rua XV"}
		}, "uf e obrigatoria quando address e informado"},
		{"address with uf", func(p *ProspectorParams) { p.Address = &Address{Street: "Rua XV"} }, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := ProspectorParams{Keyword: "padarias", City: "Curitiba", UF: "PR"}
			tc.mutate(&params)

			err := params.Validate()

			if tc.want == "" {
				require.NoError(t, err)
				return
			}

			var appErr *AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, http.StatusBadRequest, appErr.Status)
			require.Equal(t, CodeInvalidInput, appErr.Code)
			require.Equal(t, tc.want, appErr.Message)
		})
	}
}

func TestGeocodingErrorConstructors(t *testing.T) {
	cause := errors.New("boom")

	cases := []struct {
		name    string
		err     *AppError
		status  int
		code    string
		message string
	}{
		{"city", CityNotFound(), http.StatusBadRequest, "CITY_NOT_FOUND", "Cidade nao encontrada"},
		{"address", AddressNotFound(), http.StatusBadRequest, "ADDRESS_NOT_FOUND", "Endereco nao encontrado"},
		{"upstream", UpstreamUnavailable(cause), http.StatusBadGateway, "UPSTREAM_UNAVAILABLE", "Servico de consulta indisponivel"},
		{"not configured", ServiceNotConfigured(), http.StatusServiceUnavailable, "SERVICE_NOT_CONFIGURED", "API Brasil nao configurada"},
		{"cep without city", CEPWithoutCity(), http.StatusBadRequest, "CEP_NOT_FOUND", "CEP sem cidade cadastrada"},
		{"unavailable", GeocoderUnavailable(cause), http.StatusBadGateway, "GEOCODER_UNAVAILABLE", "Servico de geocodificacao indisponivel"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.status, tc.err.Status)
			require.Equal(t, tc.code, tc.err.Code)
			require.Equal(t, tc.message, tc.err.Message)
		})
	}

	require.ErrorIs(t, GeocoderUnavailable(cause), cause)
	require.ErrorIs(t, UpstreamUnavailable(cause), cause)
}
