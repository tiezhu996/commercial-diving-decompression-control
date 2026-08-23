package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"gorm.io/gorm"
)

func newScoringEngine008(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:router-scoring-008-%d?mode=memory&cache=shared", time.Now().UnixNano()),
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
	return engine, db
}

func scoringLogin008(t *testing.T, engine *gin.Engine, username, password string) string {
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

func scoringRequest008(t *testing.T, engine *gin.Engine, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func scoringPendingAssessment008(t *testing.T, engine *gin.Engine, token string) (uint, uint) {
	t.Helper()
	createBody := `{"plan_code":"PLAN-008","diver_profile_id":1,"worksite_pressure_bar":1,"breathing_mix":{"o2":0.21,"he":0,"n2":0.79},"planned_at":"2026-08-23T10:00:00Z"}`
	rec := scoringRequest008(t, engine, token, http.MethodPost, "/api/v1/plans", createBody)
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
	pid, pv := plan.Data.ID, plan.Data.Version
	segBody := fmt.Sprintf(`{"plan_version":%d,"sequence_no":1,"depth_m":20,"duration_min":10,"ascent_rate_mmin":0,"gas_mix":{"o2":0.21,"he":0,"n2":0.79},"segment_type":"bottom","notes":""}`, pv)
	rec = scoringRequest008(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/segments", pid), segBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add segment: %d %s", rec.Code, rec.Body.String())
	}
	pv++
	runBody := fmt.Sprintf(`{"plan_version":%d}`, pv)
	rec = scoringRequest008(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/assessments/run", pid), runBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	var run struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &run)
	aid := run.Data.ID
	pv++
	submitBody := fmt.Sprintf(`{"target_status":"pending_supervisor_review","version":%d,"reason":"review this"}`, pv)
	rec = scoringRequest008(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/submit", aid), submitBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	return aid, pv + 1
}

func TestDeactivatedAccountRejected(t *testing.T) {
	engine, db := newScoringEngine008(t)
	token := scoringLogin008(t, engine, "planner", "planner123")
	if err := db.Model(&auth.User{}).Where("username = ?", "planner").Update("active", false).Error; err != nil {
		t.Fatalf("deactivate user: %v", err)
	}
	rec := scoringRequest008(t, engine, token, http.MethodGet, "/api/v1/divers", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("deactivated account: got %d, want 401", rec.Code)
	}
}

func TestPlannerCannotApprove(t *testing.T) {
	engine, _ := newScoringEngine008(t)
	token := scoringLogin008(t, engine, "planner", "planner123")
	aid, version := scoringPendingAssessment008(t, engine, token)
	approveBody := fmt.Sprintf(`{"target_status":"approved_for_training","version":%d,"reason":"approve as planner"}`, version)
	rec := scoringRequest008(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/approve", aid), approveBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("planner approve: got %d, want 403", rec.Code)
	}
}

func TestCORSDisallowedOriginNoHeader(t *testing.T) {
	engine, _ := newScoringEngine008(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/divers", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin received ACAO header %q", got)
	}
}

func TestOversizedRequestIDReplaced(t *testing.T) {
	engine, _ := newScoringEngine008(t)
	long := strings.Repeat("a", 100)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/divers", nil)
	req.Header.Set("X-Request-ID", long)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	got := rec.Header().Get("X-Request-ID")
	if got == long {
		t.Fatalf("oversized request id was echoed (%d chars)", len(got))
	}
}
