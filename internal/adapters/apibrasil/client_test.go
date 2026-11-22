package apibrasil

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

type recordedRequest struct {
	path          string
	authorization string
	contentType   string
	body          map[string]any
}

func stubServer(t *testing.T, status int, response string) (*httptest.Server, *recordedRequest) {
	t.Helper()

	recorded := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &recorded.body))
		recorded.path = r.URL.Path
		recorded.authorization = r.Header.Get("Authorization")
		recorded.contentType = r.Header.Get("Content-Type")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	return server, recorded
}

const cepStringCoordinates = `{"error":false,"data":{"cep":"80010000","logradouro":"Rua XV","bairro":{"bairro":"Centro"},"cidade":{"cidade":"Curitiba","cidade_ibge":"4106902"},"estado":"PR","latitude":"-25.4284","longitude":"-49.2733"}}`

const cepNumberCoordinates = `{"data":{"cep":"80010000","logradouro":"Rua XV","bairro":{"bairro":"Centro"},"cidade":{"cidade":"Curitiba","cidade_ibge":4106902},"estado":"PR","latitude":-25.4284,"longitude":-49.2733}}`

func TestLookupCEPSuccess(t *testing.T) {
	for name, response := range map[string]string{"string": cepStringCoordinates, "number": cepNumberCoordinates} {
		t.Run(name, func(t *testing.T) {
			server, recorded := stubServer(t, http.StatusOK, response)
			client := NewClient(server.URL, "secret-token", false)

			location, err := client.LookupCEP(context.Background(), "80010000")

			require.NoError(t, err)
			require.Equal(t, "Curitiba", location.City)
			require.Equal(t, "Centro", location.Neighborhood)
			require.Equal(t, "PR", location.State)
			require.Equal(t, "4106902", location.IBGECode)
			require.InDelta(t, -25.4284, *location.Latitude, 1e-9)
			require.InDelta(t, -49.2733, *location.Longitude, 1e-9)
			require.Equal(t, "/consulta/cep/credits", recorded.path)
			require.Equal(t, "Bearer secret-token", recorded.authorization)
			require.Equal(t, "application/json", recorded.contentType)
			require.Equal(t, "cep", recorded.body["tipo"])
			require.Equal(t, "80010000", recorded.body["cep"])
			require.NotContains(t, recorded.body, "homolog")
		})
	}
}

func TestLookupCEPWithoutCoordinates(t *testing.T) {
	server, _ := stubServer(t, http.StatusOK, `{"data":{"cep":"80010000","cidade":{"cidade":"Curitiba"},"latitude":"","longitude":null}}`)

	_, err := NewClient(server.URL, "t", false).LookupCEP(context.Background(), "80010000")

	require.ErrorIs(t, err, domain.ErrCEPNotFound)
}

func TestLookupCEPErrorWithStatusOK(t *testing.T) {
	server, _ := stubServer(t, http.StatusOK, `{"error":true,"message":"CEP nao encontrado"}`)

	_, err := NewClient(server.URL, "t", false).LookupCEP(context.Background(), "80010000")

	require.ErrorIs(t, err, domain.ErrCEPNotFound)
}

func TestLookupCEPUnknownErrorWithStatusOK(t *testing.T) {
	server, _ := stubServer(t, http.StatusOK, `{"error":true,"message":"falha interna"}`)

	_, err := NewClient(server.URL, "t", false).LookupCEP(context.Background(), "80010000")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
}

func TestLookupCEPUnavailableStatuses(t *testing.T) {
	for _, status := range []int{http.StatusPaymentRequired, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway} {
		server, _ := stubServer(t, status, `{}`)

		_, err := NewClient(server.URL, "t", false).LookupCEP(context.Background(), "80010000")

		require.ErrorIs(t, err, domain.ErrGeocoderUnavailable, status)
	}
}

func TestLookupCEPInvalidJSON(t *testing.T) {
	server, _ := stubServer(t, http.StatusOK, `not json`)

	_, err := NewClient(server.URL, "t", false).LookupCEP(context.Background(), "80010000")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
}

func TestLookupCEPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient(server.URL, "secret-token", false)
	client.httpClient.Timeout = 50 * time.Millisecond

	_, err := client.LookupCEP(context.Background(), "80010000")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
	require.NotContains(t, err.Error(), "secret-token")
	require.NotContains(t, err.Error(), server.URL)
}

func TestLookupCEPResponseTooLarge(t *testing.T) {
	server, _ := stubServer(t, http.StatusOK, `{"data":{"logradouro":"`+strings.Repeat("a", maxResponseBytes)+`"}}`)

	_, err := NewClient(server.URL, "t", false).LookupCEP(context.Background(), "80010000")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
}

func TestHomologSendsFlag(t *testing.T) {
	server, recorded := stubServer(t, http.StatusOK, cepStringCoordinates)

	_, err := NewClient(server.URL, "t", true).LookupCEP(context.Background(), "80010000")

	require.NoError(t, err)
	require.Equal(t, true, recorded.body["homolog"])
}

const cnpjSuccess = `{"error":false,"data":{"dados":{
"razao_social":"EMPRESA LTDA","nome_fantasia":"Empresa","situacao_cadastral":"ATIVA","natureza_juridica":"206-2","porte":"ME",
"cnae_codigo":"4781400","cnae_descricao":"Comercio de roupas",
"cnaes_secundarios":[{"codigo":"1","descricao":"Um"},{"codigo":"2","descricao":"Dois"}],
"enderecos":[{"logradouro":"Rua A","numero":"10","complemento":"","bairro":"Centro","cep":"80010000","cidade":"Curitiba","uf":"PR"}],
"telefones":[{"telefone":"4133334444","tipo":"fixo","whatsapp":false},{"telefone":"41999998888","tipo":"movel","whatsapp":true}],
"emails":["a@b.com"],
"socios":[{"nome":"Fulano","cargo":"Socio","documento":"***123***","data_entrada":"2020-01-01"}]}}}`

func TestLookupCNPJSuccess(t *testing.T) {
	server, recorded := stubServer(t, http.StatusOK, cnpjSuccess)

	company, err := NewClient(server.URL, "secret-token", false).LookupCNPJ(context.Background(), "11222333000181")

	require.NoError(t, err)
	require.Equal(t, "EMPRESA LTDA", company.LegalName)
	require.Equal(t, "ATIVA", company.RegistrationStatus)
	require.Equal(t, domain.CNPJActivity{Code: "4781400", Description: "Comercio de roupas"}, company.MainActivity)
	require.Len(t, company.SecondaryActivities, 2)
	require.Len(t, company.Addresses, 1)
	require.Equal(t, "PR", company.Addresses[0].State)
	require.Len(t, company.Phones, 2)
	require.True(t, company.Phones[1].WhatsApp)
	require.Equal(t, []string{"a@b.com"}, company.Emails)
	require.Len(t, company.Partners, 1)
	require.Equal(t, "/consulta/cnpj/credits", recorded.path)
	require.Equal(t, "cadastro-pj-plus", recorded.body["tipo"])
	require.Equal(t, "11222333000181", recorded.body["cnpj"])
	require.Equal(t, "Bearer secret-token", recorded.authorization)
}

func TestLookupCNPJErrorWithStatusOK(t *testing.T) {
	server, _ := stubServer(t, http.StatusOK, `{"error":true,"message":"CNPJ nao encontrado"}`)

	_, err := NewClient(server.URL, "t", false).LookupCNPJ(context.Background(), "11222333000181")

	require.ErrorIs(t, err, domain.ErrInvalidCNPJ)
}

func TestLookupCNPJInsufficientBalance(t *testing.T) {
	server, _ := stubServer(t, http.StatusPaymentRequired, `{}`)

	_, err := NewClient(server.URL, "t", false).LookupCNPJ(context.Background(), "11222333000181")

	require.ErrorIs(t, err, domain.ErrGeocoderUnavailable)
}
