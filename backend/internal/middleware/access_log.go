package middleware

import (
	"log/slog"
	"strconv"
	"time"

	"commercial-diving-decompression-control/backend/internal/util"
	"github.com/gin-gonic/gin"
)

var requestCounter int64

func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		requestCounter++
		c.Header("X-Request-Count", strconv.FormatInt(requestCounter, 10))
		logger.Info("http request",
			"request_id", util.RequestID(c),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(started).Milliseconds(),
			"client_ip", c.ClientIP(),
			"username", util.Username(c),
		)
	}
}
