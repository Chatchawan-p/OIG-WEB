package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/http/response"
)

func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.ErrorContext(c.Request.Context(), "panic recovered",
			"request_id", c.GetString("request_id"),
			"panic", fmt.Sprint(recovered),
			"stack", string(debug.Stack()),
		)
		response.Failure(c, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
	})
}
