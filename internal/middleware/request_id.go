package middleware

import (
	"net/http"
	"strings"

	"commercial-diving-decompression-control/backend/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// maxRequestIDLen bounds a client-supplied X-Request-ID so an attacker cannot
// push an arbitrarily long or malformed value through the response header and
// the audit log. A UUID is 36 chars, so 64 leaves headroom for prefixed schemes.
const maxRequestIDLen = 64

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := normalizeRequestID(c.GetHeader("X-Request-ID"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(util.ContextRequestID, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// normalizeRequestID trims surrounding whitespace, rejects values longer than
// maxRequestIDLen, and keeps only printable ASCII so the value is safe to echo
// back in a response header and store in the audit trail. An empty result
// signals the caller should mint a fresh server-side ID instead.
func normalizeRequestID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > maxRequestIDLen {
		return ""
	}
	for _, r := range trimmed {
		if r < 0x21 || r > 0x7e { // disallow control chars and non-ASCII
			return ""
		}
	}
	return trimmed
}

func CORS(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	allowAll := allowed["*"]
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		// A request is only granted cross-origin credentials when its Origin is
		// explicitly allow-listed (or the wildcard is configured). Unknown
		// origins receive no ACAO header rather than being echoed back, which
		// would let any site read authenticated responses.
		if origin != "" && (allowAll || allowed[origin]) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			if origin == "" {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			if !allowAll && !allowed[origin] {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
