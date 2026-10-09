// Package apperror defines transport-independent application errors.
package apperror

import "errors"

var (
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource conflict")
	ErrUnauthorized = errors.New("authentication required")
	ErrForbidden    = errors.New("permission denied")
	ErrInvalid      = errors.New("invalid input")
)

type ValidationError struct {
	Details []FieldError
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return "validation failed" }

func (e *ValidationError) Add(field, message string) {
	e.Details = append(e.Details, FieldError{Field: field, Message: message})
}

func (e *ValidationError) OrNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}
