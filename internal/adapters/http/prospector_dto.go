package httpadapter

import "github.com/OBrunoW/gmaps-scraper/internal/domain"

type ProspectorErrorResponse struct {
	Message string `json:"message" example:"Cidade nao encontrada"`
	Code    string `json:"code" example:"CITY_NOT_FOUND"`
}

type ProspectorAddressRequest struct {
	Street       string `json:"street" example:"Rua XV de Novembro"`
	Number       string `json:"number" example:"100"`
	Neighborhood string `json:"neighborhood" example:"Centro"`
}

type ProspectorJobRequest struct {
	Keyword  string                    `json:"keyword" example:"padarias"`
	City     string                    `json:"city" example:"Curitiba"`
	UF       string                    `json:"uf" example:"PR"`
	Address  *ProspectorAddressRequest `json:"address"`
	CEP      string                    `json:"cep" example:"80010000"`
	RadiusKm *float64                  `json:"radius_km" example:"5"`
}

func (r ProspectorJobRequest) toParams() domain.ProspectorParams {
	params := domain.ProspectorParams{
		Keyword:  r.Keyword,
		City:     r.City,
		UF:       r.UF,
		CEP:      r.CEP,
		RadiusKm: r.RadiusKm,
	}
	if r.Address != nil {
		params.Address = &domain.Address{
			Street:       r.Address.Street,
			Number:       r.Address.Number,
			Neighborhood: r.Address.Neighborhood,
		}
	}

	return params
}

type ProspectorJobCreatedResponse struct {
	JobID    string `json:"job_id" example:"kYzR8xLmNpQvWjX3"`
	Status   string `json:"status" example:"pending"`
	Keyword  string `json:"keyword" example:"padarias"`
	City     string `json:"city" example:"Curitiba"`
	RadiusKm int    `json:"radius_km" example:"5"`
}

type LeadResponse struct {
	Key      string   `json:"key"`
	Title    string   `json:"title"`
	Phone    *string  `json:"phone"`
	Website  *string  `json:"website"`
	Address  *string  `json:"address"`
	City     *string  `json:"city"`
	State    *string  `json:"state"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	Category *string  `json:"category"`
	MapsURL  *string  `json:"maps_url"`
}

type ProspectorJobStatusResponse struct {
	JobID       string          `json:"job_id" example:"kYzR8xLmNpQvWjX3"`
	Status      string          `json:"status" example:"completed"`
	Keyword     string          `json:"keyword" example:"padarias em Curitiba"`
	ResultCount int             `json:"result_count" example:"2"`
	Error       *string         `json:"error"`
	Leads       *[]LeadResponse `json:"leads,omitempty"`
}

func newProspectorJobStatusResponse(publicID string, job domain.ProspectorJob) ProspectorJobStatusResponse {
	response := ProspectorJobStatusResponse{
		JobID:       publicID,
		Status:      string(job.Status),
		Keyword:     job.Keyword,
		ResultCount: job.ResultCount,
	}
	if job.Error != "" {
		message := job.Error
		response.Error = &message
	}
	if job.Status == domain.StatusCompleted {
		leads := newLeadResponses(job.Leads)
		response.Leads = &leads
	}

	return response
}

func newLeadResponses(leads []domain.Lead) []LeadResponse {
	responses := make([]LeadResponse, 0, len(leads))
	for _, lead := range leads {
		responses = append(responses, LeadResponse(lead))
	}

	return responses
}

type CEPResponse struct {
	CEP          string   `json:"cep" example:"80010000"`
	Street       string   `json:"street" example:"Rua XV de Novembro"`
	Neighborhood string   `json:"neighborhood" example:"Centro"`
	City         string   `json:"city" example:"Curitiba"`
	State        string   `json:"state" example:"PR"`
	Latitude     *float64 `json:"latitude" example:"-25.4284"`
	Longitude    *float64 `json:"longitude" example:"-49.2733"`
}

func newCEPResponse(location domain.CEPLocation) CEPResponse {
	return CEPResponse{
		CEP:          location.CEP,
		Street:       location.Street,
		Neighborhood: location.Neighborhood,
		City:         location.City,
		State:        location.State,
		Latitude:     location.Latitude,
		Longitude:    location.Longitude,
	}
}

type CNPJActivityResponse struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type CNPJAddressResponse struct {
	Street       string `json:"street"`
	Number       string `json:"number"`
	Complement   string `json:"complement"`
	Neighborhood string `json:"neighborhood"`
	CEP          string `json:"cep"`
	City         string `json:"city"`
	State        string `json:"state"`
}

type CNPJPhoneResponse struct {
	Number   string `json:"number"`
	Type     string `json:"type"`
	WhatsApp bool   `json:"whatsapp"`
}

type CNPJPartnerResponse struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Document  string `json:"document"`
	StartDate string `json:"start_date"`
}

type CNPJResponse struct {
	CNPJ                string                 `json:"cnpj"`
	LegalName           string                 `json:"legal_name"`
	TradeName           string                 `json:"trade_name"`
	RegistrationStatus  string                 `json:"registration_status"`
	LegalNature         string                 `json:"legal_nature"`
	Size                string                 `json:"size"`
	MainActivity        CNPJActivityResponse   `json:"main_activity"`
	SecondaryActivities []CNPJActivityResponse `json:"secondary_activities"`
	Addresses           []CNPJAddressResponse  `json:"addresses"`
	Phones              []CNPJPhoneResponse    `json:"phones"`
	Emails              []string               `json:"emails"`
	Partners            []CNPJPartnerResponse  `json:"partners"`
}

func newCNPJResponse(company domain.CNPJCompany) CNPJResponse {
	response := CNPJResponse{
		CNPJ:                company.CNPJ,
		LegalName:           company.LegalName,
		TradeName:           company.TradeName,
		RegistrationStatus:  company.RegistrationStatus,
		LegalNature:         company.LegalNature,
		Size:                company.Size,
		MainActivity:        CNPJActivityResponse(company.MainActivity),
		SecondaryActivities: make([]CNPJActivityResponse, 0, len(company.SecondaryActivities)),
		Addresses:           make([]CNPJAddressResponse, 0, len(company.Addresses)),
		Phones:              make([]CNPJPhoneResponse, 0, len(company.Phones)),
		Emails:              make([]string, 0, len(company.Emails)),
		Partners:            make([]CNPJPartnerResponse, 0, len(company.Partners)),
	}
	for _, activity := range company.SecondaryActivities {
		response.SecondaryActivities = append(response.SecondaryActivities, CNPJActivityResponse(activity))
	}
	for _, address := range company.Addresses {
		response.Addresses = append(response.Addresses, CNPJAddressResponse(address))
	}
	for _, phone := range company.Phones {
		response.Phones = append(response.Phones, CNPJPhoneResponse(phone))
	}
	response.Emails = append(response.Emails, company.Emails...)
	for _, partner := range company.Partners {
		response.Partners = append(response.Partners, CNPJPartnerResponse(partner))
	}

	return response
}
