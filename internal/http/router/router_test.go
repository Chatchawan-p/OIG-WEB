package router

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/config"
)

type checkerStub struct{}

func (checkerStub) Ping(context.Context) error { return nil }

func testConfig(swagger bool) config.Config {
	return config.Config{
		AppEnv:             "test",
		CORSAllowedOrigins: []string{"https://app.example.com"},
		SwaggerEnabled:     swagger,
	}
}

func TestRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name    string
		swagger bool
		path    string
		want    int
	}{
		{name: "health", path: "/health", want: http.StatusOK},
		{name: "missing route", path: "/api/v1/not-implemented", want: http.StatusNotFound},
		{name: "swagger disabled", path: "/swagger/index.html", want: http.StatusNotFound},
		{name: "swagger enabled", swagger: true, path: "/swagger/index.html", want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			New(testConfig(tt.swagger), log, checkerStub{}).ServeHTTP(recorder, request)
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.want, recorder.Body.String())
			}
		})
	}
}
