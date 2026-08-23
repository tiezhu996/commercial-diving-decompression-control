package repository

import (
	"context"
	"testing"
	"time"

	"commercial-diving-decompression-control/backend/internal/audit"
	"commercial-diving-decompression-control/backend/internal/constants"
	"commercial-diving-decompression-control/backend/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openScoringDB005(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:repo-scoring-" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.DiverProfile{}, &model.DivePlan{}, &model.ExposureSegment{}, &model.DecompressionAssessment{}, &audit.Event{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedPlan005(t *testing.T, db *gorm.DB, status constants.PlanStatus, version uint) model.DivePlan {
	t.Helper()
	plan := model.DivePlan{PlanCode: "SCORE-005", DiverProfileID: 1, WorksitePressureBar: 1, BreathingMixJSON: `{"o2":0.21,"he":0,"n2":0.79}`, PlanStatus: status, CreatedBy: 1, Version: version, PlannedAt: time.Now()}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	return plan
}

func seedAssessment005(t *testing.T, db *gorm.DB, planID uint, status string) model.DecompressionAssessment {
	t.Helper()
	item := model.DecompressionAssessment{PlanID: planID, AssessmentStatus: status, AlgorithmVersion: "training-compartment-v1", InputSnapshotJSON: "{}", CompartmentLoadsJSON: "[]", RiskFlagsJSON: "[]", HighestRiskBand: constants.RiskInformational, ComparativeScore: 100, AssumptionsJSON: "{}", CreatedAt: time.Now()}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed assessment: %v", err)
	}
	return item
}

func TestTransitionCommitsOnSuccess(t *testing.T) {
	db := openScoringDB005(t)
	repo := NewDecompressionAssessmentRepository(db, audit.NewRepository(db))
	plan := seedPlan005(t, db, constants.PlanModeled, 1)
	assessment := seedAssessment005(t, db, plan.ID, string(constants.PlanModeled))
	entry := audit.Entry{RequestID: "req-t1", ActorID: 1, Action: "decompression_assessment.submit_review", EntityType: "decompression_assessment", BeforeSummary: string(constants.PlanModeled), AfterSummary: string(constants.PlanPendingReview)}
	if err := repo.Transition(context.Background(), plan, assessment, constants.PlanPendingReview, 1, entry); err != nil {
		t.Fatalf("transition: %v", err)
	}
	var gotPlan model.DivePlan
	if err := db.First(&gotPlan, plan.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotPlan.PlanStatus != constants.PlanPendingReview {
		t.Fatalf("plan status = %s, want pending_supervisor_review", gotPlan.PlanStatus)
	}
	var gotAssessment model.DecompressionAssessment
	if err := db.First(&gotAssessment, assessment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotAssessment.AssessmentStatus != string(constants.PlanPendingReview) {
		t.Fatalf("assessment status = %s, want pending_supervisor_review", gotAssessment.AssessmentStatus)
	}
}

func TestTransitionRejectsStaleAssessmentState(t *testing.T) {
	db := openScoringDB005(t)
	repo := NewDecompressionAssessmentRepository(db, audit.NewRepository(db))
	plan := seedPlan005(t, db, constants.PlanModeled, 1)
	assessment := seedAssessment005(t, db, plan.ID, string(constants.PlanModeled))
	stale := assessment
	stale.AssessmentStatus = string(constants.PlanPendingReview)
	entry := audit.Entry{RequestID: "req-t2", ActorID: 1, Action: "decompression_assessment.submit_review", EntityType: "decompression_assessment", BeforeSummary: string(constants.PlanModeled), AfterSummary: string(constants.PlanPendingReview)}
	if err := repo.Transition(context.Background(), plan, stale, constants.PlanPendingReview, 1, entry); err == nil {
		t.Fatal("expected conflict for stale assessment state")
	}
	var gotPlan model.DivePlan
	if err := db.First(&gotPlan, plan.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotPlan.PlanStatus != constants.PlanModeled {
		t.Fatalf("plan must stay modeled after rejected transition, got %s", gotPlan.PlanStatus)
	}
}

func TestCreateModeledConflictRollsBack(t *testing.T) {
	db := openScoringDB005(t)
	repo := NewDecompressionAssessmentRepository(db, audit.NewRepository(db))
	plan := seedPlan005(t, db, constants.PlanDraft, 1)
	item := model.DecompressionAssessment{PlanID: plan.ID, AssessmentStatus: string(constants.PlanModeled), AlgorithmVersion: "training-compartment-v1", InputSnapshotJSON: "{}", CompartmentLoadsJSON: "[]", RiskFlagsJSON: "[]", HighestRiskBand: constants.RiskInformational, ComparativeScore: 100, AssumptionsJSON: "{}"}
	stalePlan := plan
	stalePlan.Version = plan.Version + 1
	entry := audit.Entry{RequestID: "req-t3", ActorID: 1, Action: "decompression_assessment.run", EntityType: "decompression_assessment", BeforeSummary: "draft", AfterSummary: "modeled"}
	if err := repo.CreateModeled(context.Background(), stalePlan, &item, entry); err == nil {
		t.Fatal("expected conflict for stale plan version")
	}
	var gotPlan model.DivePlan
	if err := db.First(&gotPlan, plan.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotPlan.PlanStatus != constants.PlanDraft {
		t.Fatalf("plan must stay draft after conflict, got %s", gotPlan.PlanStatus)
	}
	if gotPlan.Version != plan.Version {
		t.Fatalf("plan version changed after conflict: got %d, want %d", gotPlan.Version, plan.Version)
	}
	var assessmentCount int64
	db.Model(&model.DecompressionAssessment{}).Where("plan_id = ?", plan.ID).Count(&assessmentCount)
	if assessmentCount != 0 {
		t.Fatalf("assessment created despite conflict: %d", assessmentCount)
	}
}

