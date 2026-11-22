package domain

import (
	"errors"
	"net/http"
)

const (
	CodeInvalidInput = "invalid_input"
	CodeNotFound     = "not_found"
	CodeInternal     = "internal"
)

type AppError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func InvalidInput(msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: CodeInvalidInput, Message: msg}
}

func NotFound(msg string) *AppError {
	return &AppError{Status: http.StatusNotFound, Code: CodeNotFound, Message: msg}
}

func Internal(msg string, err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Code: CodeInternal, Message: msg, Err: err}
}

const (
	CodeCityNotFound         = "CITY_NOT_FOUND"
	CodeAddressNotFound      = "ADDRESS_NOT_FOUND"
	CodeGeocoderUnavailable  = "GEOCODER_UNAVAILABLE"
	CodeUpstreamUnavailable  = "UPSTREAM_UNAVAILABLE"
	CodeServiceNotConfigured = "SERVICE_NOT_CONFIGURED"
)

var (
	ErrLocationNotFound    = errors.New("location not found")
	ErrGeocoderUnavailable = errors.New("geocoder unavailable")
)

func CityNotFound() *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: CodeCityNotFound, Message: "Cidade nao encontrada"}
}

func AddressNotFound() *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: CodeAddressNotFound, Message: "Endereco nao encontrado"}
}

func GeocoderUnavailable(err error) *AppError {
	return &AppError{
		Status:  http.StatusBadGateway,
		Code:    CodeGeocoderUnavailable,
		Message: "Servico de geocodificacao indisponivel",
		Err:     err,
	}
}

func UpstreamUnavailable(err error) *AppError {
	return &AppError{
		Status:  http.StatusBadGateway,
		Code:    CodeUpstreamUnavailable,
		Message: "Servico de consulta indisponivel",
		Err:     err,
	}
}

func ServiceNotConfigured() *AppError {
	return &AppError{
		Status:  http.StatusServiceUnavailable,
		Code:    CodeServiceNotConfigured,
		Message: "API Brasil nao configurada",
	}
}
