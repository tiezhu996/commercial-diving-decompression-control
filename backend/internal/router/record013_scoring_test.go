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

func newScoringEngine007(t *testing.T) *gin.Engine {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:router-scoring-007-%d?mode=memory&cache=shared", time.Now().UnixNano()),
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

func scoringLogin007(t *testing.T, engine *gin.Engine, username, password string) string {
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

func scoringRequest007(t *testing.T, engine *gin.Engine, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func scoringPlanID007(t *testing.T, rec *httptest.ResponseRecorder) (uint, uint) {
	t.Helper()
	var envelope struct {
		Data struct {
			ID      uint `json:"id"`
			Version uint `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	return envelope.Data.ID, envelope.Data.Version
}

func scoringAssessmentID007(t *testing.T, rec *httptest.ResponseRecorder) uint {
	t.Helper()
	var envelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	return envelope.Data.ID
}

func scoringMakePendingPlan007(t *testing.T, engine *gin.Engine, token string, code string) (planID, assessmentID, version uint) {
	t.Helper()
	createBody := fmt.Sprintf(`{"plan_code":%q,"diver_profile_id":1,"worksite_pressure_bar":1,"breathing_mix":{"o2":0.21,"he":0,"n2":0.79},"planned_at":"2026-08-23T10:00:00Z"}`, code)
	rec := scoringRequest007(t, engine, token, http.MethodPost, "/api/v1/plans", createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create plan: %d %s", rec.Code, rec.Body.String())
	}
	planID, version = scoringPlanID007(t, rec)
	segmentBody := fmt.Sprintf(`{"plan_version":%d,"sequence_no":1,"depth_m":20,"duration_min":10,"ascent_rate_mmin":0,"gas_mix":{"o2":0.21,"he":0,"n2":0.79},"segment_type":"bottom","notes":""}`, version)
	rec = scoringRequest007(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/segments", planID), segmentBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add segment: %d %s", rec.Code, rec.Body.String())
	}
	version++
	runBody := fmt.Sprintf(`{"plan_version":%d}`, version)
	rec = scoringRequest007(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/assessments/run", planID), runBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("run assessment: %d %s", rec.Code, rec.Body.String())
	}
	assessmentID = scoringAssessmentID007(t, rec)
	version++
	submitBody := fmt.Sprintf(`{"target_status":"pending_supervisor_review","version":%d,"reason":"please review this plan"}`, version)
	rec = scoringRequest007(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/submit", assessmentID), submitBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit assessment: %d %s", rec.Code, rec.Body.String())
	}
	version++
	return planID, assessmentID, version
}

func TestApproveFromModeledRejected(t *testing.T) {
	engine := newScoringEngine007(t)
	planner := scoringLogin007(t, engine, "planner", "planner123")
	supervisor := scoringLogin007(t, engine, "supervisor", "supervisor123")
	createBody := `{"plan_code":"PLAN-007-M","diver_profile_id":1,"worksite_pressure_bar":1,"breathing_mix":{"o2":0.21,"he":0,"n2":0.79},"planned_at":"2026-08-23T10:00:00Z"}`
	rec := scoringRequest007(t, engine, planner, http.MethodPost, "/api/v1/plans", createBody)
	pid, pv := scoringPlanID007(t, rec)
	segBody := fmt.Sprintf(`{"plan_version":%d,"sequence_no":1,"depth_m":20,"duration_min":10,"ascent_rate_mmin":0,"gas_mix":{"o2":0.21,"he":0,"n2":0.79},"segment_type":"bottom","notes":""}`, pv)
	rec = scoringRequest007(t, engine, planner, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/segments", pid), segBody)
	pv++
	runBody := fmt.Sprintf(`{"plan_version":%d}`, pv)
	rec = scoringRequest007(t, engine, planner, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/assessments/run", pid), runBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	aid := scoringAssessmentID007(t, rec)
	pv++
	approveBody := fmt.Sprintf(`{"target_status":"approved_for_training","version":%d,"reason":"approve directly"}`, pv)
	rec = scoringRequest007(t, engine, supervisor, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/approve", aid), approveBody)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("approve from modeled: got %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArchiveFromPendingRejected(t *testing.T) {
	engine := newScoringEngine007(t)
	planner := scoringLogin007(t, engine, "planner", "planner123")
	supervisor := scoringLogin007(t, engine, "supervisor", "supervisor123")
	planID, _, version := scoringMakePendingPlan007(t, engine, planner, "PLAN-007-A")
	archiveBody := fmt.Sprintf(`{"target_status":"archived","version":%d,"reason":"archive it"}`, version)
	rec := scoringRequest007(t, engine, supervisor, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/archive", planID), archiveBody)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("archive from pending: got %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestApproveRequiresReviewRole(t *testing.T) {
	engine := newScoringEngine007(t)
	token := scoringLogin007(t, engine, "planner", "planner123")
	_, assessmentID, version := scoringMakePendingPlan007(t, engine, token, "PLAN-007-R")
	approveBody := fmt.Sprintf(`{"target_status":"approved_for_training","version":%d,"reason":"approve as planner"}`, version)
	rec := scoringRequest007(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/approve", assessmentID), approveBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("planner approve: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArchiveRequiresReviewRole(t *testing.T) {
	engine := newScoringEngine007(t)
	planner := scoringLogin007(t, engine, "planner", "planner123")
	planID, _, version := scoringMakePendingPlan007(t, engine, planner, "PLAN-007-AR")
	archiveBody := fmt.Sprintf(`{"target_status":"archived","version":%d,"reason":"archive as planner"}`, version)
	rec := scoringRequest007(t, engine, planner, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/archive", planID), archiveBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("planner archive: got %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}
