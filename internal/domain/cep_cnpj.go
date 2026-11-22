package domain

import (
	"errors"
	"net/http"
)

const (
	CodeCEPNotFound = "CEP_NOT_FOUND"
	CodeInvalidCNPJ = "INVALID_CNPJ"
)

var (
	ErrCEPNotFound = errors.New("cep not found")
	ErrInvalidCNPJ = errors.New("invalid cnpj")
)

func CEPNotFound() *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: CodeCEPNotFound, Message: "CEP nao encontrado"}
}

func CEPWithoutCity() *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: CodeCEPNotFound, Message: "CEP sem cidade cadastrada"}
}

func InvalidCNPJ() *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: CodeInvalidCNPJ, Message: "CNPJ invalido"}
}

type CEPLocation struct {
	CEP          string
	Street       string
	Neighborhood string
	City         string
	State        string
	IBGECode     string
	Latitude     *float64
	Longitude    *float64
}

type CNPJActivity struct {
	Code        string
	Description string
}

type CNPJAddress struct {
	Street       string
	Number       string
	Complement   string
	Neighborhood string
	CEP          string
	City         string
	State        string
}

type CNPJPhone struct {
	Number   string
	Type     string
	WhatsApp bool
}

type CNPJPartner struct {
	Name      string
	Role      string
	Document  string
	StartDate string
}

type CNPJCompany struct {
	CNPJ                string
	LegalName           string
	TradeName           string
	RegistrationStatus  string
	LegalNature         string
	Size                string
	MainActivity        CNPJActivity
	SecondaryActivities []CNPJActivity
	Addresses           []CNPJAddress
	Phones              []CNPJPhone
	Emails              []string
	Partners            []CNPJPartner
}
