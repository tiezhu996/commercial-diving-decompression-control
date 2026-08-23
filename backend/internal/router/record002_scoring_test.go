package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"commercial-diving-decompression-control/backend/internal/audit"
	"commercial-diving-decompression-control/backend/internal/auth"
	"commercial-diving-decompression-control/backend/internal/config"
	"commercial-diving-decompression-control/backend/internal/database"
	"commercial-diving-decompression-control/backend/internal/handler"
	"commercial-diving-decompression-control/backend/internal/middleware"
	"commercial-diving-decompression-control/backend/internal/repository"
	"commercial-diving-decompression-control/backend/internal/router"
	"commercial-diving-decompression-control/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func newScoringEngine(t *testing.T) *gin.Engine {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:router-scoring-%d?mode=memory&cache=shared", time.Now().UnixNano()),
		DBAutoMigrate: true, JWTSecret: "scoring-jwt-secret-at-least-32-bytes-long",
		JWTTTL: time.Hour, CORSOrigins: []string{"http://localhost:18526"},
		RateLimitPerMinute: 10000, ModelVersion: "training-compartment-v1", MaxSegments: 24,
		LogLevel: "error", GracefulShutdownPeriod: time.Second,
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	auditRepo := audit.NewRepository(db)
	authRepo := auth.NewRepository(db)
	profileRepo := repository.NewDiverProfileRepository(db, auditRepo)
	planRepo := repository.NewDivePlanRepository(db, auditRepo)
	segmentRepo := repository.NewExposureSegmentRepository(db, auditRepo)
	assessmentRepo := repository.NewDecompressionAssessmentRepository(db, auditRepo)

	authService := auth.NewService(authRepo, cfg.JWTSecret, cfg.JWTTTL)
	profileService := service.NewDiverProfileService(profileRepo, planRepo)
	planService := service.NewDivePlanService(planRepo, profileRepo)
	segmentService := service.NewExposureSegmentService(segmentRepo, planRepo, cfg.MaxSegments)
	assessmentService := service.NewDecompressionAssessmentService(assessmentRepo, planRepo, profileRepo, segmentRepo, cfg.ModelVersion, cfg.MaxSegments)

	engine := gin.New()
	engine.Use(middleware.RequestID())
	engine.Use(middleware.Recovery(logger))
	engine.Use(middleware.AccessLog(logger))
	engine.Use(middleware.CORS(cfg.CORSOrigins))
	engine.Use(middleware.NewLocalRateLimiter(cfg.RateLimitPerMinute).Handler())
	api := engine.Group("/api/v1")
	api.POST("/auth/login", auth.NewHandler(authService).Login)
	protected := api.Group("")
	protected.Use(middleware.Auth(authService))
	write := middleware.RBAC(auth.RolePlanner, auth.RoleAdmin)
	review := middleware.RBAC(auth.RoleSupervisor, auth.RoleAdmin)
	router.RegisterDiverProfileRoutes(protected, handler.NewDiverProfileHandler(profileService), write)
	router.RegisterDivePlanRoutes(protected, handler.NewDivePlanHandler(planService), write, review)
	router.RegisterExposureSegmentRoutes(protected, handler.NewExposureSegmentHandler(segmentService), write)
	router.RegisterDecompressionAssessmentRoutes(protected, handler.NewDecompressionAssessmentHandler(assessmentService), write, review)
	protected.GET("/audit-events", review, audit.NewHandler(auditRepo).List)
	return engine
}

func scoringLogin(t *testing.T, engine *gin.Engine, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return envelope.Data.Token
}

func scoringGet(t *testing.T, engine *gin.Engine, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestMissingDiverReturns404(t *testing.T) {
	engine := newScoringEngine(t)
	token := scoringLogin(t, engine, "planner", "planner123")
	rec := scoringGet(t, engine, token, "/api/v1/divers/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing diver: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMissingPlanReturns404(t *testing.T) {
	engine := newScoringEngine(t)
	token := scoringLogin(t, engine, "planner", "planner123")
	rec := scoringGet(t, engine, token, "/api/v1/plans/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing plan: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMissingSegmentReturns404(t *testing.T) {
	engine := newScoringEngine(t)
	token := scoringLogin(t, engine, "planner", "planner123")
	rec := scoringGet(t, engine, token, "/api/v1/segments/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing segment: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestMissingPlanSegmentCreate404(t *testing.T) {
	engine := newScoringEngine(t)
	token := scoringLogin(t, engine, "planner", "planner123")
	body := bytes.NewBufferString(`{"plan_version":1,"sequence_no":1,"depth_m":10,"duration_min":2,"ascent_rate_mmin":0,"gas_mix":{"o2":0.21,"he":0,"n2":0.79},"segment_type":"bottom","notes":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans/999999/segments", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("create segment on missing plan: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}
