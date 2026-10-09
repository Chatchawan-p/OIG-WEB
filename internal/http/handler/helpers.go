package handler

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/repository"
)

func validationError(field, message string) error {
	return &apperror.ValidationError{Details: []apperror.FieldError{{Field: field, Message: message}}}
}

func pageFromQuery(c *gin.Context, defaultLimit, maxLimit int) (repository.Page, error) {
	page := 1
	limit := defaultLimit
	var err error
	if value := strings.TrimSpace(c.Query("page")); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil || page < 1 {
			return repository.Page{}, validationError("page", "must be a positive integer")
		}
	}
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > maxLimit {
			return repository.Page{}, validationError("limit", fmt.Sprintf("must be between 1 and %d", maxLimit))
		}
	}
	return repository.Page{Number: page, Limit: limit}, nil
}

func parseGenerationRange(value string) (*int, *int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil, nil
	}
	parts := strings.Split(value, "-")
	if len(parts) > 2 {
		return nil, nil, validationError("genRangeFilter", "must be a generation or min-max")
	}
	min, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || min < 1 {
		return nil, nil, validationError("genRangeFilter", "must be a generation or min-max")
	}
	max := min
	if len(parts) == 2 {
		max, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || max < min {
			return nil, nil, validationError("genRangeFilter", "maximum must be greater than or equal to minimum")
		}
	}
	return &min, &max, nil
}

func allowedReturnURL(raw string, origins []string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.User != nil {
		return "", false
	}
	if strings.HasPrefix(parsed.Path, "/api/v1/auth/discord/callback") {
		return "", false
	}
	origin := parsed.Scheme + "://" + parsed.Host
	for _, allowed := range origins {
		if origin == allowed {
			return parsed.String(), true
		}
	}
	return "", false
}
