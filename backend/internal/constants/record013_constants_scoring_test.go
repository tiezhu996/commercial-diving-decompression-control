package constants

import "testing"

func TestUnapproveTransitionRejected(t *testing.T) {
	if CanTransitionPlan(PlanApprovedTraining, PlanDraft) {
		t.Fatal("approved_for_training must not transition back to draft")
	}
	if CanTransitionPlan(PlanModeled, PlanApprovedTraining) {
		t.Fatal("modeled must not transition directly to approved_for_training")
	}
	if CanTransitionPlan(PlanPendingReview, PlanArchived) {
		t.Fatal("pending_supervisor_review must not transition directly to archived")
	}
}

func TestSelfTransitionRejected(t *testing.T) {
	if CanTransitionPlan(PlanPendingReview, PlanPendingReview) {
		t.Fatal("a status must not transition to itself")
	}
}
