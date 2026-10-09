package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/http/response"
)

func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		if _, ok := allowed[origin]; !ok {
			response.Failure(c, http.StatusForbidden, "origin is not allowed", "CORS_ORIGIN_FORBIDDEN", nil)
			c.Abort()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Header("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			requestedMethod := strings.ToUpper(c.GetHeader("Access-Control-Request-Method"))
			if requestedMethod == "" {
				response.Failure(c, http.StatusBadRequest, "missing preflight method", "INVALID_CORS_PREFLIGHT", nil)
				c.Abort()
				return
			}
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}
