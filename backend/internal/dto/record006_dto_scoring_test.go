package dto

import (
	"testing"

	"commercial-diving-decompression-control/backend/internal/model"
)

func TestSegmentResponseGasErrorNotZero(t *testing.T) {
	item := model.ExposureSegment{ID: 1, PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 10, GasMixJSON: `{"o2":0.05,"n2":0.95}`, SegmentType: "bottom"}
	response, err := NewExposureSegmentResponse(item)
	if err == nil {
		t.Fatalf("NewExposureSegmentResponse swallowed invalid gas and returned zero response %+v", response)
	}
}

func TestPlanResponseGasErrorNotZero(t *testing.T) {
	item := model.DivePlan{ID: 1, PlanCode: "SCORE-006", BreathingMixJSON: `{"o2":0.05,"n2":0.95}`}
	response, err := NewDivePlanResponse(item, "SCORE-D")
	if err == nil {
		t.Fatalf("NewDivePlanResponse swallowed invalid gas and returned zero response %+v", response)
	}
}
