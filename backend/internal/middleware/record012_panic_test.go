package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPanicReturnsErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := gin.New()
	engine.Use(RequestID())
	engine.Use(Recovery(logger))
	engine.GET("/boom", func(c *gin.Context) { panic("boom") })
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic handler: got status %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("panic handler returned an empty body")
	}
}
