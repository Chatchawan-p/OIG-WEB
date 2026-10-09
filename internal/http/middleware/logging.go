package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func Logging(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		attributes := []any{
			"request_id", c.GetString("request_id"),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(started).Milliseconds(),
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		}
		if last := c.Errors.Last(); last != nil {
			attributes = append(attributes, "error", last.Err)
			log.ErrorContext(c.Request.Context(), "http request", attributes...)
			return
		}
		log.InfoContext(c.Request.Context(), "http request", attributes...)
	}
}
