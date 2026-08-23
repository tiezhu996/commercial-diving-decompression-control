package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"commercial-diving-decompression-control/backend/internal/util"
	"github.com/gin-gonic/gin"
)

type visitorWindow struct {
	started time.Time
	count   int
}

type LocalRateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	visitors map[string]visitorWindow
}

func NewLocalRateLimiter(limit int) *LocalRateLimiter {
	return NewLocalRateLimiterWithWindow(limit, time.Minute)
}

// NewLocalRateLimiterWithWindow constructs a limiter with an explicit window,
// primarily so callers (and tests) can exercise the window-reset path without
// waiting a full minute.
func NewLocalRateLimiterWithWindow(limit int, window time.Duration) *LocalRateLimiter {
	return &LocalRateLimiter{limit: limit, window: window, visitors: map[string]visitorWindow{}}
}

func (l *LocalRateLimiter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		key := c.ClientIP()
		l.mu.Lock()
		state, exists := l.visitors[key]
		// Fixed window: reset the counter once the previous window has elapsed
		// so a client is never stuck over the limit after the window passes.
		if !exists || now.Sub(state.started) >= l.window {
			state = visitorWindow{started: now}
		}
		state.count++
		// All map access happens under the lock; writing back outside the lock
		// caused a data race and lost updates that let the counter drift below
		// the real request rate.
		l.visitors[key] = state
		remaining := l.limit - state.count
		if remaining < 0 {
			remaining = 0
		}
		reset := state.started.Add(l.window)
		blocked := state.count > l.limit
		l.mu.Unlock()

		c.Header("X-RateLimit-Limit", strconv.Itoa(l.limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		if blocked {
			c.Header("Retry-After", strconv.Itoa(int(time.Until(reset).Seconds())+1))
			c.Abort()
			c.JSON(http.StatusTooManyRequests, util.Envelope{
				Error:     &util.Error{Code: "RATE_LIMITED", Message: "local request limit exceeded"},
				RequestID: util.RequestID(c),
			})
			return
		}
		c.Next()
	}
}
