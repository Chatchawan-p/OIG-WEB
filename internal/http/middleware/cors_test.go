package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		origin string
		want   int
	}{
		{name: "allowed", origin: "https://app.example.com", want: http.StatusOK},
		{name: "rejected", origin: "https://evil.example.com", want: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CORS([]string{"https://app.example.com"}))
			router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Origin", tt.origin)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d", response.Code, tt.want)
			}
		})
	}
}
