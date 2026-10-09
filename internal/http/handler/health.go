// Package handler contains HTTP-only parsing and response mapping.
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/http/response"
)

type HealthChecker interface {
	Ping(context.Context) error
}

type HealthHandler struct {
	checker HealthChecker
}

type HealthData struct {
	Status string `json:"status" example:"up"`
}

func NewHealthHandler(checker HealthChecker) *HealthHandler {
	return &HealthHandler{checker: checker}
}

// Health godoc
// @Summary Liveness probe
// @Description Reports whether the API process is running.
// @Tags System
// @Produce json
// @Success 200 {object} response.Envelope{data=HealthData}
// @Router /health [get]
func (h *HealthHandler) Health(c *gin.Context) {
	response.Success(c, http.StatusOK, "service is healthy", HealthData{Status: "up"})
}

// Ready godoc
// @Summary Readiness probe
// @Description Reports whether required dependencies are reachable.
// @Tags System
// @Produce json
// @Success 200 {object} response.Envelope{data=HealthData}
// @Failure 503 {object} response.Envelope
// @Router /ready [get]
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.checker.Ping(ctx); err != nil {
		response.Failure(c, http.StatusServiceUnavailable, "service is not ready", "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	response.Success(c, http.StatusOK, "service is ready", HealthData{Status: "ready"})
}
