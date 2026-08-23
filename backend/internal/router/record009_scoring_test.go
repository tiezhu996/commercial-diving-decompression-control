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

func newScoringEngine009(t *testing.T) *gin.Engine {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:router-scoring-009-%d?mode=memory&cache=shared", time.Now().UnixNano()),
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
	return engine
}

func scoringLogin009(t *testing.T, engine *gin.Engine) string {
	return scoringLoginUser009(t, engine, "planner", "planner123")
}

func scoringLoginUser009(t *testing.T, engine *gin.Engine, username, password string) string {
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
	_ = json.Unmarshal(rec.Body.Bytes(), &envelope)
	return envelope.Data.Token
}

func scoringGet009(t *testing.T, engine *gin.Engine, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestDiverGetErrorNotSwallowed(t *testing.T) {
	engine := newScoringEngine009(t)
	token := scoringLogin009(t, engine)
	rec := scoringGet009(t, engine, token, "/api/v1/divers/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing diver: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPlanGetErrorNotSwallowed(t *testing.T) {
	engine := newScoringEngine009(t)
	token := scoringLogin009(t, engine)
	rec := scoringGet009(t, engine, token, "/api/v1/plans/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing plan: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestSegmentGetErrorNotSwallowed(t *testing.T) {
	engine := newScoringEngine009(t)
	token := scoringLogin009(t, engine)
	rec := scoringGet009(t, engine, token, "/api/v1/segments/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing segment: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAssessmentGetErrorNotSwallowed(t *testing.T) {
	engine := newScoringEngine009(t)
	token := scoringLogin009(t, engine)
	rec := scoringGet009(t, engine, token, "/api/v1/assessments/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing assessment: got %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPlanArchiveErrorNotSwallowed(t *testing.T) {
	engine := newScoringEngine009(t)
	token := scoringLogin009(t, engine)
	supervisor := scoringLoginUser009(t, engine, "supervisor", "supervisor123")
	_ = supervisor
	// create a draft plan directly via API
	createBody := `{"plan_code":"PLAN-009-AR","diver_profile_id":1,"worksite_pressure_bar":1,"breathing_mix":{"o2":0.21,"he":0,"n2":0.79},"planned_at":"2026-08-23T10:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create plan: %d %s", rec.Code, rec.Body.String())
	}
	var plan struct {
		Data struct {
			ID      uint `json:"id"`
			Version uint `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &plan)
	archiveBody := fmt.Sprintf(`{"target_status":"archived","version":%d,"reason":"archive draft"}`, plan.Data.Version)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/archive", plan.Data.ID), bytes.NewBufferString(archiveBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+supervisor)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("archive draft plan: got %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestSegmentCreateErrorNotSwallowed(t *testing.T) {
	engine := newScoringEngine009(t)
	token := scoringLogin009(t, engine)
	createBody := `{"plan_code":"PLAN-009-S","diver_profile_id":1,"worksite_pressure_bar":1,"breathing_mix":{"o2":0.21,"he":0,"n2":0.79},"planned_at":"2026-08-23T10:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create plan: %d %s", rec.Code, rec.Body.String())
	}
	var plan struct {
		Data struct {
			ID      uint `json:"id"`
			Version uint `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &plan)
	segBody := fmt.Sprintf(`{"plan_version":%d,"sequence_no":1,"depth_m":20,"duration_min":10,"ascent_rate_mmin":0,"gas_mix":{"o2":0.05,"he":0,"n2":0.95},"segment_type":"bottom","notes":""}`, plan.Data.Version)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/segments", plan.Data.ID), bytes.NewBufferString(segBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create segment with invalid gas: got %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
}
