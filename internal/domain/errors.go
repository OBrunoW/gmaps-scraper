package domain

import "net/http"

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
