package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type checkerStub struct{ err error }

func (s checkerStub) Ping(context.Context) error { return s.err }

func TestHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/health", nil)
	NewHealthHandler(checkerStub{}).Health(ctx)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"up"`) {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
	_ = router
}

func TestReadyWhenMongoDBUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	NewHealthHandler(checkerStub{err: errors.New("unavailable")}).Ready(ctx)

	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
	_ = router
}
