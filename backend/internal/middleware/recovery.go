package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"commercial-diving-decompression-control/backend/internal/util"
	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("request panic recovered", "request_id", util.RequestID(c), "error", recovered, "stack", string(debug.Stack()))
				// Abort the request so the panic does not surface as the default 200 with an empty body.
				c.AbortWithStatusJSON(http.StatusInternalServerError, util.Envelope{Error: &util.Error{Code: "INTERNAL_ERROR", Message: "internal service error"}, RequestID: util.RequestID(c)})
			}
		}()
		c.Next()
	}
}
