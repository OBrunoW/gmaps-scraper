package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/middleware"
)

type fakeProspector struct {
	created     application.CreatedProspectorJob
	createErr   error
	gotParams   domain.ProspectorParams
	job         domain.ProspectorJob
	getErr      error
	gotPublicID string
}

func (f *fakeProspector) Create(_ context.Context, params domain.ProspectorParams) (application.CreatedProspectorJob, error) {
	f.gotParams = params
	return f.created, f.createErr
}

func (f *fakeProspector) Get(_ context.Context, publicID string) (domain.ProspectorJob, error) {
	f.gotPublicID = publicID
	return f.job, f.getErr
}

type fakeCEPLookup struct {
	location domain.CEPLocation
	err      error
	gotCEP   string
}

func (f *fakeCEPLookup) Lookup(_ context.Context, cep string) (domain.CEPLocation, error) {
	f.gotCEP = cep
	return f.location, f.err
}

type fakeCNPJLookup struct {
	company domain.CNPJCompany
	err     error
	gotCNPJ string
}

func (f *fakeCNPJLookup) Lookup(_ context.Context, cnpj string) (domain.CNPJCompany, error) {
	f.gotCNPJ = cnpj
	return f.company, f.err
}

type prospectorEnv struct {
	prospector *fakeProspector
	ceps       *fakeCEPLookup
	cnpjs      *fakeCNPJLookup
	router     http.Handler
}

func newProspectorEnv(auth func(http.Handler) http.Handler) *prospectorEnv {
	env := &prospectorEnv{
		prospector: &fakeProspector{},
		ceps:       &fakeCEPLookup{},
		cnpjs:      &fakeCNPJLookup{},
	}
	if auth == nil {
		auth = middleware.NoopAuth
	}
	router := chi.NewRouter()
	Routes(router, nil, NewProspectorHandlers(env.prospector, env.ceps, env.cnpjs), auth)
	env.router = router
	return env
}

func (e *prospectorEnv) do(method, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	e.router.ServeHTTP(recorder, request)
	return recorder
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json %q: %v", recorder.Body.String(), err)
	}
	return body
}

func floatPointer(value float64) *float64 { return &value }

func textPointer(value string) *string { return &value }

func TestProspectorCreateAccepted(t *testing.T) {
	env := newProspectorEnv(nil)
	env.prospector.created = application.CreatedProspectorJob{
		PublicID: "pub1", Status: domain.StatusPending, Keyword: "padarias", City: "Curitiba", RadiusKm: 10,
	}
	recorder := env.do(http.MethodPost, "/api/v1/prospector/jobs",
		`{"keyword":"padarias","city":"Curitiba","uf":"PR","radius_km":10}`)
	assertJSON(t, recorder, 202, `{"job_id":"pub1","status":"pending","keyword":"padarias","city":"Curitiba","radius_km":10}`)
}

func TestProspectorCreateMapsBodyToParams(t *testing.T) {
	env := newProspectorEnv(nil)
	env.do(http.MethodPost, "/api/v1/prospector/jobs",
		`{"keyword":"k","city":"c","uf":"PR","cep":"80010000","radius_km":0,"address":{"street":"s","number":"1","neighborhood":"n"}}`)

	params := env.prospector.gotParams
	if params.Keyword != "k" || params.City != "c" || params.UF != "PR" || params.CEP != "80010000" {
		t.Fatalf("params = %+v", params)
	}
	if params.RadiusKm == nil || *params.RadiusKm != 0 {
		t.Fatalf("radius = %v, want explicit 0", params.RadiusKm)
	}
	if params.Address == nil || *params.Address != (domain.Address{Street: "s", Number: "1", Neighborhood: "n"}) {
		t.Fatalf("address = %+v", params.Address)
	}
}

func TestProspectorCreateAbsentRadiusIsNil(t *testing.T) {
	env := newProspectorEnv(nil)
	env.do(http.MethodPost, "/api/v1/prospector/jobs", `{"keyword":"k","city":"c"}`)
	if env.prospector.gotParams.RadiusKm != nil || env.prospector.gotParams.Address != nil {
		t.Fatalf("params = %+v", env.prospector.gotParams)
	}
}

func TestProspectorCreateCEPAndCityTogetherForwardsBoth(t *testing.T) {
	env := newProspectorEnv(nil)
	env.do(http.MethodPost, "/api/v1/prospector/jobs", `{"keyword":"k","city":"Curitiba","uf":"PR","cep":"80010000"}`)
	params := env.prospector.gotParams
	if params.CEP != "80010000" || params.City != "Curitiba" {
		t.Fatalf("params = %+v", params)
	}
}

func TestProspectorCreateInvalidBody(t *testing.T) {
	bodies := []string{
		`{`,
		``,
		`{"keyword":1}`,
		`{"keyword":"k","address":"rua"}`,
		`{"keyword":"k","address":[1]}`,
		`{"keyword":"k","radius_km":"x"}`,
	}
	for _, body := range bodies {
		env := newProspectorEnv(nil)
		recorder := env.do(http.MethodPost, "/api/v1/prospector/jobs", body)
		assertJSON(t, recorder, 400, `{"message":"invalid request body","code":"BAD_REQUEST"}`)
	}
}

func TestProspectorErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"validation", domain.InvalidInput("keyword deve ter entre 2 e 120 caracteres"), 400,
			`{"message":"keyword deve ter entre 2 e 120 caracteres","code":"BAD_REQUEST"}`},
		{"uf required", domain.InvalidInput("uf e obrigatoria quando address e informado"), 400,
			`{"message":"uf e obrigatoria quando address e informado","code":"BAD_REQUEST"}`},
		{"city", domain.CityNotFound(), 400, `{"message":"Cidade nao encontrada","code":"CITY_NOT_FOUND"}`},
		{"address", domain.AddressNotFound(), 400, `{"message":"Endereco nao encontrado","code":"ADDRESS_NOT_FOUND"}`},
		{"cep", domain.CEPNotFound(), 400, `{"message":"CEP nao encontrado","code":"CEP_NOT_FOUND"}`},
		{"cnpj", domain.InvalidCNPJ(), 400, `{"message":"CNPJ invalido","code":"INVALID_CNPJ"}`},
		{"geocoder", domain.GeocoderUnavailable(errors.New("boom")), 502,
			`{"message":"Servico de geocodificacao indisponivel","code":"GEOCODER_UNAVAILABLE"}`},
		{"upstream", domain.UpstreamUnavailable(errors.New("boom")), 502,
			`{"message":"Servico de consulta indisponivel","code":"UPSTREAM_UNAVAILABLE"}`},
		{"not configured", domain.ServiceNotConfigured(), 503,
			`{"message":"API Brasil nao configurada","code":"SERVICE_NOT_CONFIGURED"}`},
		{"cep without city", domain.CEPWithoutCity(), 400,
			`{"message":"CEP sem cidade cadastrada","code":"CEP_NOT_FOUND"}`},
		{"not found", domain.NotFound("job not found"), 404, `{"message":"job not found","code":"NOT_FOUND"}`},
		{"internal", domain.Internal("failed to create job", errors.New("db")), 500,
			`{"message":"failed to create job","code":"INTERNAL_ERROR"}`},
		{"untyped", errors.New("surprise"), 500, `{"message":"internal error","code":"INTERNAL_ERROR"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newProspectorEnv(nil)
			env.prospector.createErr = tc.err
			assertJSON(t, env.do(http.MethodPost, "/api/v1/prospector/jobs", `{"keyword":"k","city":"c"}`), tc.status, tc.body)
		})
	}
}

func TestProspectorGetErrorMapping(t *testing.T) {
	env := newProspectorEnv(nil)
	env.prospector.getErr = domain.NotFound("job not found")
	assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/jobs/zzz", ""), 404,
		`{"message":"job not found","code":"NOT_FOUND"}`)

	env.prospector.getErr = domain.Internal("failed to read job results", errors.New("bad"))
	assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/jobs/zzz", ""), 500,
		`{"message":"failed to read job results","code":"INTERNAL_ERROR"}`)
}

func TestProspectorGetPendingOmitsLeads(t *testing.T) {
	env := newProspectorEnv(nil)
	env.prospector.job = domain.ProspectorJob{PublicID: "internal", Status: domain.StatusRunning, Keyword: "padarias em Curitiba"}
	recorder := env.do(http.MethodGet, "/api/v1/prospector/jobs/abc", "")
	assertJSON(t, recorder, 200, `{"job_id":"abc","status":"running","keyword":"padarias em Curitiba","result_count":0,"error":null}`)
	if env.prospector.gotPublicID != "abc" {
		t.Fatalf("public id = %q", env.prospector.gotPublicID)
	}
}

func TestProspectorGetFailedHasError(t *testing.T) {
	env := newProspectorEnv(nil)
	env.prospector.job = domain.ProspectorJob{Status: domain.StatusFailed, Keyword: "k", Error: "job timed out with no results"}
	assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/jobs/abc", ""), 200,
		`{"job_id":"abc","status":"failed","keyword":"k","result_count":0,"error":"job timed out with no results"}`)
}

func TestProspectorGetCompletedWithoutLeadsIsEmptyArray(t *testing.T) {
	for _, leads := range [][]domain.Lead{nil, {}} {
		env := newProspectorEnv(nil)
		env.prospector.job = domain.ProspectorJob{Status: domain.StatusCompleted, Keyword: "k", Leads: leads}
		assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/jobs/abc", ""), 200,
			`{"job_id":"abc","status":"completed","keyword":"k","result_count":0,"error":null,"leads":[]}`)
	}
}

func TestProspectorGetCompletedLeadContract(t *testing.T) {
	env := newProspectorEnv(nil)
	env.prospector.job = domain.ProspectorJob{
		Status: domain.StatusCompleted, Keyword: "k", ResultCount: 2,
		Leads: []domain.Lead{
			{Key: "place:1", Title: "Padaria", Phone: textPointer("+55"), Lat: floatPointer(-25.4), Lng: floatPointer(-49.2)},
			{Key: "link:2", Title: "Sem dados"},
		},
	}
	body := decodeBody(t, env.do(http.MethodGet, "/api/v1/prospector/jobs/abc", ""))
	leads := body["leads"].([]any)
	if len(leads) != 2 {
		t.Fatalf("leads = %v", leads)
	}
	first := leads[0].(map[string]any)
	keys := []string{"key", "title", "phone", "website", "address", "city", "state", "lat", "lng", "category", "maps_url"}
	if len(first) != len(keys) {
		t.Fatalf("lead keys = %v", first)
	}
	for _, key := range keys {
		if _, ok := first[key]; !ok {
			t.Errorf("lead missing key %q", key)
		}
	}
	if first["lat"].(float64) != -25.4 || first["phone"].(string) != "+55" || first["website"] != nil {
		t.Fatalf("lead = %v", first)
	}
	second := leads[1].(map[string]any)
	if second["lat"] != nil || second["lng"] != nil || second["phone"] != nil {
		t.Fatalf("second lead = %v", second)
	}
	if body["result_count"].(float64) != 2 {
		t.Fatalf("result_count = %v", body["result_count"])
	}
}

func TestProspectorCEPContract(t *testing.T) {
	env := newProspectorEnv(nil)
	env.ceps.location = domain.CEPLocation{
		CEP: "80010000", Street: "Rua XV", Neighborhood: "Centro", City: "Curitiba", State: "PR",
		Latitude: floatPointer(-25.4), Longitude: floatPointer(-49.2),
	}
	recorder := env.do(http.MethodGet, "/api/v1/prospector/cep/80010-000", "")
	assertJSON(t, recorder, 200,
		`{"cep":"80010000","street":"Rua XV","neighborhood":"Centro","city":"Curitiba","state":"PR","latitude":-25.4,"longitude":-49.2}`)
	if env.ceps.gotCEP != "80010-000" {
		t.Fatalf("cep = %q", env.ceps.gotCEP)
	}
}

func TestProspectorCEPWithoutCoordinatesIsNull(t *testing.T) {
	env := newProspectorEnv(nil)
	env.ceps.location = domain.CEPLocation{CEP: "80010000"}
	body := decodeBody(t, env.do(http.MethodGet, "/api/v1/prospector/cep/80010000", ""))
	if body["latitude"] != nil || body["longitude"] != nil {
		t.Fatalf("body = %v", body)
	}
}

func TestProspectorCEPErrors(t *testing.T) {
	env := newProspectorEnv(nil)
	env.ceps.err = domain.CEPNotFound()
	assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/cep/00000000", ""), 400,
		`{"message":"CEP nao encontrado","code":"CEP_NOT_FOUND"}`)

	env.ceps.err = domain.InvalidInput("cep deve ter 8 digitos")
	assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/cep/12", ""), 400,
		`{"message":"cep deve ter 8 digitos","code":"BAD_REQUEST"}`)

	env.ceps.err = domain.GeocoderUnavailable(errors.New("down"))
	assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/cep/80010000", ""), 502,
		`{"message":"Servico de geocodificacao indisponivel","code":"GEOCODER_UNAVAILABLE"}`)
}

func TestProspectorCNPJContract(t *testing.T) {
	env := newProspectorEnv(nil)
	env.cnpjs.company = domain.CNPJCompany{
		CNPJ: "11222333000181", LegalName: "Empresa Ltda", TradeName: "Empresa",
		RegistrationStatus: "ATIVA", LegalNature: "206-2", Size: "ME",
		MainActivity:        domain.CNPJActivity{Code: "4721102", Description: "Padaria"},
		SecondaryActivities: []domain.CNPJActivity{{Code: "1", Description: "x"}},
		Addresses:           []domain.CNPJAddress{{Street: "Rua", Number: "1", City: "Curitiba", State: "PR"}},
		Phones:              []domain.CNPJPhone{{Number: "4133330000", Type: "fixo", WhatsApp: true}},
		Emails:              []string{"a@b.com"},
		Partners:            []domain.CNPJPartner{{Name: "Ana", Role: "Socia", Document: "***", StartDate: "2020-01-01"}},
	}
	body := decodeBody(t, env.do(http.MethodGet, "/api/v1/prospector/cnpj/11222333000181", ""))
	if body["cnpj"] != "11222333000181" || body["legal_name"] != "Empresa Ltda" || body["trade_name"] != "Empresa" ||
		body["registration_status"] != "ATIVA" || body["legal_nature"] != "206-2" || body["size"] != "ME" {
		t.Fatalf("body = %v", body)
	}
	if body["main_activity"].(map[string]any)["code"] != "4721102" {
		t.Fatalf("main_activity = %v", body["main_activity"])
	}
	for _, key := range []string{"secondary_activities", "addresses", "phones", "emails", "partners"} {
		if list, ok := body[key].([]any); !ok || len(list) != 1 {
			t.Errorf("%s = %v", key, body[key])
		}
	}
	phone := body["phones"].([]any)[0].(map[string]any)
	if phone["whatsapp"] != true || phone["number"] != "4133330000" {
		t.Fatalf("phone = %v", phone)
	}
	if partner := body["partners"].([]any)[0].(map[string]any); partner["start_date"] != "2020-01-01" {
		t.Fatalf("partner = %v", partner)
	}
}

func TestProspectorCNPJEmptyListsAreArrays(t *testing.T) {
	env := newProspectorEnv(nil)
	body := decodeBody(t, env.do(http.MethodGet, "/api/v1/prospector/cnpj/11222333000181", ""))
	for _, key := range []string{"secondary_activities", "addresses", "phones", "emails", "partners"} {
		if list, ok := body[key].([]any); !ok || len(list) != 0 {
			t.Errorf("%s = %v", key, body[key])
		}
	}
}

func TestProspectorCNPJInvalid(t *testing.T) {
	env := newProspectorEnv(nil)
	env.cnpjs.err = domain.InvalidCNPJ()
	assertJSON(t, env.do(http.MethodGet, "/api/v1/prospector/cnpj/123", ""), 400,
		`{"message":"CNPJ invalido","code":"INVALID_CNPJ"}`)
}

func TestProspectorCEPAndCNPJWithoutTokenReturn503(t *testing.T) {
	router := chi.NewRouter()
	Routes(router, nil, NewProspectorHandlers(&fakeProspector{}, nil, nil), middleware.NoopAuth)
	for _, path := range []string{"/api/v1/prospector/cep/80010000", "/api/v1/prospector/cnpj/11222333000181"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		assertJSON(t, recorder, 503, `{"message":"API Brasil nao configurada","code":"SERVICE_NOT_CONFIGURED"}`)
	}
}

func TestProspectorJobsWorkWithoutAPIBrasil(t *testing.T) {
	fake := &fakeProspector{created: application.CreatedProspectorJob{PublicID: "p", Status: domain.StatusPending}}
	router := chi.NewRouter()
	Routes(router, nil, NewProspectorHandlers(fake, nil, nil), middleware.NoopAuth)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/prospector/jobs", strings.NewReader(`{"keyword":"k","city":"c"}`)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestProspectorRoutesAreProtected(t *testing.T) {
	env := newProspectorEnv(middleware.NewServiceAuth(map[string]string{"k": "svc"}, "").Handler)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/prospector/jobs"},
		{http.MethodGet, "/api/v1/prospector/jobs/abc"},
		{http.MethodGet, "/api/v1/prospector/cep/80010000"},
		{http.MethodGet, "/api/v1/prospector/cnpj/11222333000181"},
	}
	for _, route := range routes {
		recorder := env.do(route.method, route.path, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d", route.method, route.path, recorder.Code)
		}
		if contentType := recorder.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
			t.Fatalf("content type = %q", contentType)
		}
	}
}

func TestProspectorRouteAcceptsValidKey(t *testing.T) {
	env := newProspectorEnv(middleware.NewServiceAuth(map[string]string{"k": "svc"}, "").Handler)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/prospector/jobs/abc", nil)
	request.Header.Set("X-Service-Key", "k")
	recorder := httptest.NewRecorder()
	env.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestToolsServedWithoutAuth(t *testing.T) {
	env := newProspectorEnv(middleware.NewServiceAuth(map[string]string{"k": "svc"}, "").Handler)
	for _, path := range []string{"/tools/", "/tools/scrape.html", "/tools/prospector.html", "/tools/lead-learning.html"} {
		recorder := env.do(http.MethodGet, path, "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, recorder.Code)
		}
		if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Fatalf("%s content type = %q", path, contentType)
		}
	}
}

func TestExistingProspectorEnvRouterStillServesScrape(t *testing.T) {
	env := newTestEnv(nil)
	recorder := do(env, http.MethodGet, "/api/v1/jobs", "", nil)
	assertJSON(t, recorder, 200, `{"jobs":[],"has_more":false}`)
}

func TestProspectorCreateWithCEPWithoutTokenReturns503(t *testing.T) {
	fake := &fakeProspector{createErr: domain.ServiceNotConfigured()}
	router := chi.NewRouter()
	Routes(router, nil, NewProspectorHandlers(fake, nil, nil), middleware.NoopAuth)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/prospector/jobs", strings.NewReader(`{"keyword":"k","cep":"80010000"}`)))
	assertJSON(t, recorder, 503, `{"message":"API Brasil nao configurada","code":"SERVICE_NOT_CONFIGURED"}`)
	if fake.gotParams.CEP != "80010000" {
		t.Fatalf("cep = %q", fake.gotParams.CEP)
	}
}
