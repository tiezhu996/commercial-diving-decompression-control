package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
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

func newScoringEngine010(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	cfg := config.Config{
		Port: "0", DBDriver: "sqlite",
		DBDSN: fmt.Sprintf("file:router-scoring-010-%d?mode=memory&cache=shared", time.Now().UnixNano()),
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

func scoringLogin010(t *testing.T, engine *gin.Engine) string {
	return scoringLoginUser010(t, engine, "planner", "planner123")
}

func scoringLoginUser010(t *testing.T, engine *gin.Engine, username, password string) string {
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

func scoringRequest010(t *testing.T, engine *gin.Engine, token, method, path, body string) *httptest.ResponseRecorder {
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

func scoringCreatePlan010(t *testing.T, engine *gin.Engine, token, code string) (uint, uint) {
	t.Helper()
	body := fmt.Sprintf(`{"plan_code":%q,"diver_profile_id":1,"worksite_pressure_bar":1,"breathing_mix":{"o2":0.21,"he":0,"n2":0.79},"planned_at":"2026-08-23T10:00:00Z"}`, code)
	rec := scoringRequest010(t, engine, token, http.MethodPost, "/api/v1/plans", body)
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
	return plan.Data.ID, plan.Data.Version
}

func scoringAddSegment010(t *testing.T, engine *gin.Engine, token string, pid, version uint) uint {
	t.Helper()
	segBody := fmt.Sprintf(`{"plan_version":%d,"sequence_no":1,"depth_m":20,"duration_min":10,"ascent_rate_mmin":0,"gas_mix":{"o2":0.21,"he":0,"n2":0.79},"segment_type":"bottom","notes":""}`, version)
	rec := scoringRequest010(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/segments", pid), segBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add segment: %d %s", rec.Code, rec.Body.String())
	}
	return version + 1
}

func scoringRun010(t *testing.T, engine *gin.Engine, token string, pid, version uint) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"plan_version":%d}`, version)
	return scoringRequest010(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/plans/%d/assessments/run", pid), body)
}

func scoringPending010(t *testing.T, engine *gin.Engine, token, code string) (pid, aid, version uint) {
	t.Helper()
	pid, version = scoringCreatePlan010(t, engine, token, code)
	version = scoringAddSegment010(t, engine, token, pid, version)
	rec := scoringRun010(t, engine, token, pid, version)
	if rec.Code != http.StatusCreated {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	var run struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &run)
	aid = run.Data.ID
	version++
	submitBody := fmt.Sprintf(`{"target_status":"pending_supervisor_review","version":%d,"reason":"please review"}`, version)
	rec = scoringRequest010(t, engine, token, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/submit", aid), submitBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	return pid, aid, version + 1
}

func TestRunRejectsStaleVersion(t *testing.T) {
	engine, _ := newScoringEngine010(t)
	token := scoringLogin010(t, engine)
	pid, version := scoringCreatePlan010(t, engine, token, "PLAN-010-R")
	version = scoringAddSegment010(t, engine, token, pid, version)
	stale := version - 1
	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := scoringRun010(t, engine, token, pid, stale)
			results <- rec.Code
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for code := range results {
		if code != http.StatusConflict {
			t.Fatalf("stale-version run returned %d, want all requests rejected with 409", code)
		}
	}
}

func TestApproveRejectsStaleVersion(t *testing.T) {
	engine, _ := newScoringEngine010(t)
	token := scoringLogin010(t, engine)
	supervisor := scoringLoginUser010(t, engine, "supervisor", "supervisor123")
	_, aid, version := scoringPending010(t, engine, token, "PLAN-010-AS")
	stale := version - 1
	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			body := fmt.Sprintf(`{"target_status":"approved_for_training","version":%d,"reason":"approve stale"}`, stale)
			rec := scoringRequest010(t, engine, supervisor, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/approve", aid), body)
			results <- rec.Code
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for code := range results {
		if code != http.StatusConflict {
			t.Fatalf("stale-version approve returned %d, want all requests rejected with 409", code)
		}
	}
}

func TestApproveRejectsDesyncedState(t *testing.T) {
	engine, db := newScoringEngine010(t)
	token := scoringLogin010(t, engine)
	supervisor := scoringLoginUser010(t, engine, "supervisor", "supervisor123")
	_, aid, version := scoringPending010(t, engine, token, "PLAN-010-DS")
	// force an inconsistent state: assessment still modeled while plan is pending review
	if err := db.Exec("UPDATE decompression_assessments SET assessment_status = 'modeled' WHERE id = ?", aid).Error; err != nil {
		t.Fatalf("seed desync: %v", err)
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			body := fmt.Sprintf(`{"target_status":"approved_for_training","version":%d,"reason":"approve desynced"}`, version)
			rec := scoringRequest010(t, engine, supervisor, http.MethodPost, fmt.Sprintf("/api/v1/assessments/%d/approve", aid), body)
			results <- rec.Code
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for code := range results {
		if code != http.StatusConflict {
			t.Fatalf("desynced approve returned %d, want all requests rejected with 409", code)
		}
	}
}

func TestRunRequiresPositiveVersion(t *testing.T) {
	engine, _ := newScoringEngine010(t)
	token := scoringLogin010(t, engine)
	pid, version := scoringCreatePlan010(t, engine, token, "PLAN-010-P")
	version = scoringAddSegment010(t, engine, token, pid, version)
	_ = version
	rec := scoringRun010(t, engine, token, pid, 0)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("run with zero version: got %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}
