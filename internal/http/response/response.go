// Package response contains the public API response envelope.
package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/apperror"
)

type Envelope struct {
	Status  int        `json:"status" example:"200"`
	Message string     `json:"message" example:"ok"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code" example:"VALIDATION_ERROR"`
	Details any    `json:"details,omitempty"`
}

func Success(c *gin.Context, status int, message string, data any) {
	c.JSON(status, Envelope{Status: status, Message: message, Data: data})
}

func Failure(c *gin.Context, status int, message, code string, details any) {
	c.JSON(status, Envelope{
		Status:  status,
		Message: message,
		Error:   &ErrorBody{Code: code, Details: details},
	})
}

func FromError(c *gin.Context, err error) {
	var validation *apperror.ValidationError
	switch {
	case errors.As(err, &validation):
		Failure(c, http.StatusBadRequest, "invalid request", "VALIDATION_ERROR", validation.Details)
	case errors.Is(err, apperror.ErrInvalid):
		Failure(c, http.StatusBadRequest, "invalid request", "VALIDATION_ERROR", nil)
	case errors.Is(err, apperror.ErrUnauthorized):
		Failure(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED", nil)
	case errors.Is(err, apperror.ErrForbidden):
		Failure(c, http.StatusForbidden, "permission denied", "FORBIDDEN", nil)
	case errors.Is(err, apperror.ErrNotFound):
		Failure(c, http.StatusNotFound, "resource not found", "NOT_FOUND", nil)
	case errors.Is(err, apperror.ErrConflict):
		Failure(c, http.StatusConflict, "resource conflict", "CONFLICT", nil)
	default:
		_ = c.Error(err)
		Failure(c, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
	}
}
