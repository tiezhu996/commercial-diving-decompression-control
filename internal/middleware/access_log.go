package middleware

import (
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"commercial-diving-decompression-control/backend/internal/util"
	"github.com/gin-gonic/gin"
)

var requestCounter atomic.Int64

func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		// Stamp the counter before c.Next() so the header is part of the
		// committed response — once the handler writes the body, headers set
		// afterward never reach the client (and would all read as empty).
		c.Header("X-Request-Count", strconv.FormatInt(requestCounter.Add(1), 10))
		c.Next()
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
