package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newScoringLimiter(limit int, window time.Duration) *LocalRateLimiter {
	return &LocalRateLimiter{limit: limit, window: window, visitors: map[string]visitorWindow{}}
}

func scoringEngine(limiter *LocalRateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(limiter.Handler())
	engine.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	return engine
}

func doRequest(engine *gin.Engine, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ip
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestConcurrentSameIPRespectsLimit(t *testing.T) {
	limiter := newScoringLimiter(1, time.Minute)
	engine := scoringEngine(limiter)

	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- doRequest(engine, "203.0.113.9:12345").Code
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	allowed := 0
	for code := range results {
		if code == http.StatusOK {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("expected exactly 1 allowed request under concurrency, got %d", allowed)
	}
}

func TestRateLimitRecoversAfterWindow(t *testing.T) {
	limiter := newScoringLimiter(1, 500*time.Millisecond)
	engine := scoringEngine(limiter)

	if code := doRequest(engine, "198.51.100.7:1111").Code; code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", code)
	}
	if code := doRequest(engine, "198.51.100.7:1111").Code; code != http.StatusTooManyRequests {
		t.Fatalf("second request should be limited, got %d", code)
	}
	time.Sleep(600 * time.Millisecond)
	if code := doRequest(engine, "198.51.100.7:1111").Code; code != http.StatusOK {
		t.Fatalf("request after window expiry should pass, got %d", code)
	}
}

func TestRateLimitBoundary(t *testing.T) {
	limiter := newScoringLimiter(2, time.Minute)
	engine := scoringEngine(limiter)

	expected := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests}
	for i, want := range expected {
		if got := doRequest(engine, "192.0.2.44:2222").Code; got != want {
			t.Fatalf("request %d: got %d, want %d", i+1, got, want)
		}
	}
}

func TestAccessLogRequestCountUnique(t *testing.T) {
	engine := gin.New()
	engine.Use(AccessLog(slog.New(slog.NewTextHandler(io.Discard, nil))))
	engine.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	const workers = 20
	start := make(chan struct{})
	counts := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			counts <- rec.Header().Get("X-Request-Count")
		}()
	}
	close(start)
	wg.Wait()
	close(counts)

	seen := map[string]bool{}
	for value := range counts {
		if value == "" {
			t.Fatal("missing X-Request-Count header")
		}
		if seen[value] {
			t.Fatalf("duplicate request count %q observed", value)
		}
		seen[value] = true
	}
}
