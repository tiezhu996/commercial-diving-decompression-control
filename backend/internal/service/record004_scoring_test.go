package service

import (
	"context"
	"testing"
	"time"

	"commercial-diving-decompression-control/backend/internal/audit"
	"commercial-diving-decompression-control/backend/internal/dto"
	"commercial-diving-decompression-control/backend/internal/model"
	"commercial-diving-decompression-control/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openScoringDB004(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:service-scoring-004-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.DiverProfile{}, &model.DivePlan{}, &model.ExposureSegment{}, &model.DecompressionAssessment{}, &audit.Event{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func seedDiver004(t *testing.T, db *gorm.DB) model.DiverProfile {
	t.Helper()
	profile := model.DiverProfile{ProfileCode: "SCORE-004", DisplayName: "Scoring Diver", QualificationLevel: "commercial", DefaultO2Fraction: 0.21, DefaultHeFraction: 0, ProfileStatus: "active", Version: 1, CreatedAt: time.Now()}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("seed diver: %v", err)
	}
	return profile
}

func TestDiverGetHonorsCanceledCtx(t *testing.T) {
	db := openScoringDB004(t)
	profile := seedDiver004(t, db)
	auditRepo := audit.NewRepository(db)
	svc := NewDiverProfileService(repository.NewDiverProfileRepository(db, auditRepo), repository.NewDivePlanRepository(db, auditRepo))
	if _, err := svc.Get(canceledCtx(), profile.ID); err == nil {
		t.Fatal("DiverProfileService.Get executed work under a canceled context")
	}
}

func TestDiverListHonorsCanceledCtx(t *testing.T) {
	db := openScoringDB004(t)
	auditRepo := audit.NewRepository(db)
	svc := NewDiverProfileService(repository.NewDiverProfileRepository(db, auditRepo), repository.NewDivePlanRepository(db, auditRepo))
	if _, _, err := svc.List(canceledCtx(), "", "", 1, 50); err == nil {
		t.Fatal("DiverProfileService.List executed work under a canceled context")
	}
}

func TestPlanListHonorsCanceledCtx(t *testing.T) {
	db := openScoringDB004(t)
	auditRepo := audit.NewRepository(db)
	svc := NewDivePlanService(repository.NewDivePlanRepository(db, auditRepo), repository.NewDiverProfileRepository(db, auditRepo))
	if _, _, err := svc.List(canceledCtx(), "", "", "", 1, 50); err == nil {
		t.Fatal("DivePlanService.List executed work under a canceled context")
	}
}


func TestSegmentGetHonorsCanceledCtx(t *testing.T) {
	db := openScoringDB004(t)
	mix := `{"o2":0.21,"he":0,"n2":0.79}`
	segment := model.ExposureSegment{PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 10, GasMixJSON: mix, SegmentType: "bottom", CreatedAt: time.Now()}
	if err := db.Create(&segment).Error; err != nil {
		t.Fatalf("seed segment: %v", err)
	}
	auditRepo := audit.NewRepository(db)
	svc := NewExposureSegmentService(repository.NewExposureSegmentRepository(db, auditRepo), repository.NewDivePlanRepository(db, auditRepo), 24)
	if _, err := svc.Get(canceledCtx(), segment.ID); err == nil {
		t.Fatal("ExposureSegmentService.Get executed work under a canceled context")
	}
}

func TestDiverCreateHonorsCanceledCtx(t *testing.T) {
	db := openScoringDB004(t)
	auditRepo := audit.NewRepository(db)
	svc := NewDiverProfileService(repository.NewDiverProfileRepository(db, auditRepo), repository.NewDivePlanRepository(db, auditRepo))
	req := dto.CreateDiverProfileRequest{ProfileCode: "SCORE-C", DisplayName: "Create Diver", QualificationLevel: "commercial", DefaultO2Fraction: 0.21, DefaultHeFraction: 0, ProfileStatus: "active"}
	if _, err := svc.Create(canceledCtx(), req, audit.Entry{RequestID: "req-c", ActorID: 1}); err == nil {
		t.Fatal("DiverProfileService.Create executed work under a canceled context")
	}
}
