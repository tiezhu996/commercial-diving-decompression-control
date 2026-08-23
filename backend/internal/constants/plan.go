package constants

type PlanStatus string

const (
	PlanDraft            PlanStatus = "draft"
	PlanModeled          PlanStatus = "modeled"
	PlanPendingReview    PlanStatus = "pending_supervisor_review"
	PlanApprovedTraining PlanStatus = "approved_for_training"
	PlanArchived         PlanStatus = "archived"
)

var planTransitions = map[PlanStatus]map[PlanStatus]bool{
	PlanDraft:            {PlanModeled: true},
	PlanModeled:          {PlanPendingReview: true},
	PlanPendingReview:    {PlanApprovedTraining: true},
	PlanApprovedTraining: {PlanArchived: true},
	PlanArchived:         {},
}

func ValidPlanStatus(status PlanStatus) bool {
	_, ok := planTransitions[status]
	return ok
}

func CanTransitionPlan(from, to PlanStatus) bool {
	if from == to {
		return false
	}
	return planTransitions[from][to]
}

func PlanStatuses() []PlanStatus {
	return []PlanStatus{PlanDraft, PlanModeled, PlanPendingReview, PlanApprovedTraining, PlanArchived}
}
