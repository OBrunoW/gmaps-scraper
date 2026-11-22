package apibrasil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
)

const (
	DefaultBaseURL = "https://gateway.apibrasil.io/api/v2"

	requestTimeout   = 10 * time.Second
	maxResponseBytes = 1 << 20
	cepPath          = "/consulta/cep/credits"
	cnpjPath         = "/consulta/cnpj/credits"
)

type Client struct {
	baseURL    string
	token      string
	homolog    bool
	httpClient *http.Client
}

func NewClient(baseURL, token string, homolog bool) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		homolog:    homolog,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

func (c *Client) LookupCEP(ctx context.Context, cep string) (domain.CEPLocation, error) {
	body := map[string]any{"tipo": "cep", "cep": cep}

	var envelope cepEnvelope
	if err := c.post(ctx, cepPath, body, &envelope, domain.ErrCEPNotFound); err != nil {
		return domain.CEPLocation{}, err
	}

	if err := envelope.failure(domain.ErrCEPNotFound); err != nil {
		return domain.CEPLocation{}, err
	}

	location := envelope.Data.toDomain()
	if location.Latitude == nil || location.Longitude == nil {
		return domain.CEPLocation{}, domain.ErrCEPNotFound
	}

	return location, nil
}

func (c *Client) LookupCNPJ(ctx context.Context, cnpj string) (domain.CNPJCompany, error) {
	body := map[string]any{"tipo": "cadastro-pj-plus", "cnpj": cnpj}

	var envelope cnpjEnvelope
	if err := c.post(ctx, cnpjPath, body, &envelope, domain.ErrInvalidCNPJ); err != nil {
		return domain.CNPJCompany{}, err
	}

	if err := envelope.failure(domain.ErrInvalidCNPJ); err != nil {
		return domain.CNPJCompany{}, err
	}

	if envelope.Data.Dados == nil {
		return domain.CNPJCompany{}, domain.ErrInvalidCNPJ
	}

	company := envelope.Data.Dados.toDomain()
	company.CNPJ = cnpj

	return company, nil
}

func (c *Client) post(ctx context.Context, path string, body map[string]any, target any, notFound error) error {
	if c.homolog {
		body["homolog"] = true
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: encode request: %w", domain.ErrGeocoderUnavailable, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: build request: %w", domain.ErrGeocoderUnavailable, sanitizeError(err))
	}

	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: apibrasil request: %w", domain.ErrGeocoderUnavailable, sanitizeError(err))
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: read response: %w", domain.ErrGeocoderUnavailable, sanitizeError(err))
	}

	if len(raw) > maxResponseBytes {
		return fmt.Errorf("%w: response exceeds %d bytes", domain.ErrGeocoderUnavailable, maxResponseBytes)
	}

	if err := statusError(response.StatusCode, notFound); err != nil {
		return err
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: invalid json: %w", domain.ErrGeocoderUnavailable, err)
	}

	return nil
}

func statusError(status int, notFound error) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusPaymentRequired:
		return fmt.Errorf("%w: insufficient balance", domain.ErrGeocoderUnavailable)
	case status == http.StatusNotFound || status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return notFound
	default:
		return fmt.Errorf("%w: apibrasil status %d", domain.ErrGeocoderUnavailable, status)
	}
}

func sanitizeError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}

	return err
}

type apiStatus struct {
	Error   *bool  `json:"error"`
	Message string `json:"message"`
}

func (s apiStatus) failure(notFound error) error {
	if s.Error == nil || !*s.Error {
		return nil
	}

	if isNotFoundMessage(s.Message) {
		return notFound
	}

	return fmt.Errorf("%w: apibrasil error: %s", domain.ErrGeocoderUnavailable, s.Message)
}

func isNotFoundMessage(message string) bool {
	lowered := strings.ToLower(message)
	for _, marker := range []string{"encontrad", "invalid", "inválid", "inexistent", "not found"} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}

	return false
}

type flexString string

func (f *flexString) UnmarshalJSON(raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}

	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		*f = flexString(text)
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(trimmed, &number); err != nil {
		return nil
	}
	*f = flexString(number.String())

	return nil
}

type cepEnvelope struct {
	apiStatus
	Data cepData `json:"data"`
}

type cepData struct {
	CEP          flexString      `json:"cep"`
	Street       flexString      `json:"logradouro"`
	Neighborhood json.RawMessage `json:"bairro"`
	City         json.RawMessage `json:"cidade"`
	State        flexString      `json:"estado"`
	Latitude     flexString      `json:"latitude"`
	Longitude    flexString      `json:"longitude"`
}

func (d cepData) toDomain() domain.CEPLocation {
	city, ibge := cityFields(d.City)

	return domain.CEPLocation{
		CEP:          string(d.CEP),
		Street:       string(d.Street),
		Neighborhood: objectOrString(d.Neighborhood, "bairro"),
		City:         city,
		State:        string(d.State),
		IBGECode:     ibge,
		Latitude:     parseCoordinate(string(d.Latitude)),
		Longitude:    parseCoordinate(string(d.Longitude)),
	}
}

func cityFields(raw json.RawMessage) (string, string) {
	if len(raw) == 0 {
		return "", ""
	}

	var object struct {
		City flexString `json:"cidade"`
		IBGE flexString `json:"cidade_ibge"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		return string(object.City), string(object.IBGE)
	}

	return objectOrString(raw, "cidade"), ""
}

func objectOrString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var object map[string]flexString
	if err := json.Unmarshal(raw, &object); err == nil {
		return string(object[key])
	}

	return ""
}

func parseCoordinate(value string) *float64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}

	return &parsed
}

type cnpjEnvelope struct {
	apiStatus
	Data struct {
		Dados *cnpjData `json:"dados"`
	} `json:"data"`
}

type cnpjData struct {
	LegalName           flexString `json:"razao_social"`
	TradeName           flexString `json:"nome_fantasia"`
	RegistrationStatus  flexString `json:"situacao_cadastral"`
	LegalNature         flexString `json:"natureza_juridica"`
	Size                flexString `json:"porte"`
	MainCode            flexString `json:"cnae_codigo"`
	MainDescription     flexString `json:"cnae_descricao"`
	SecondaryActivities []struct {
		Code        flexString `json:"codigo"`
		Description flexString `json:"descricao"`
	} `json:"cnaes_secundarios"`
	Addresses []struct {
		Street       flexString `json:"logradouro"`
		Number       flexString `json:"numero"`
		Complement   flexString `json:"complemento"`
		Neighborhood flexString `json:"bairro"`
		CEP          flexString `json:"cep"`
		City         flexString `json:"cidade"`
		State        flexString `json:"uf"`
	} `json:"enderecos"`
	Phones []struct {
		Number   flexString `json:"telefone"`
		Type     flexString `json:"tipo"`
		WhatsApp bool       `json:"whatsapp"`
	} `json:"telefones"`
	Emails   []flexString `json:"emails"`
	Partners []struct {
		Name      flexString `json:"nome"`
		Role      flexString `json:"cargo"`
		Document  flexString `json:"documento"`
		StartDate flexString `json:"data_entrada"`
	} `json:"socios"`
}

func (d cnpjData) toDomain() domain.CNPJCompany {
	company := domain.CNPJCompany{
		LegalName:          string(d.LegalName),
		TradeName:          string(d.TradeName),
		RegistrationStatus: string(d.RegistrationStatus),
		LegalNature:        string(d.LegalNature),
		Size:               string(d.Size),
		MainActivity:       domain.CNPJActivity{Code: string(d.MainCode), Description: string(d.MainDescription)},
	}

	for _, activity := range d.SecondaryActivities {
		company.SecondaryActivities = append(company.SecondaryActivities, domain.CNPJActivity{
			Code:        string(activity.Code),
			Description: string(activity.Description),
		})
	}

	for _, address := range d.Addresses {
		company.Addresses = append(company.Addresses, domain.CNPJAddress{
			Street:       string(address.Street),
			Number:       string(address.Number),
			Complement:   string(address.Complement),
			Neighborhood: string(address.Neighborhood),
			CEP:          string(address.CEP),
			City:         string(address.City),
			State:        string(address.State),
		})
	}

	for _, phone := range d.Phones {
		company.Phones = append(company.Phones, domain.CNPJPhone{
			Number:   string(phone.Number),
			Type:     string(phone.Type),
			WhatsApp: phone.WhatsApp,
		})
	}

	for _, email := range d.Emails {
		if email != "" {
			company.Emails = append(company.Emails, string(email))
		}
	}

	for _, partner := range d.Partners {
		company.Partners = append(company.Partners, domain.CNPJPartner{
			Name:      string(partner.Name),
			Role:      string(partner.Role),
			Document:  string(partner.Document),
			StartDate: string(partner.StartDate),
		})
	}

	return company
}
