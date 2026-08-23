package decompression

import (
	"testing"

	"commercial-diving-decompression-control/backend/internal/constants"
	"commercial-diving-decompression-control/backend/internal/model"
)

func scoringPlan003() (model.DivePlan, model.DiverProfile, []model.ExposureSegment) {
	mix, _ := EncodeGasMix(GasMix{O2: 0.21, He: 0, N2: 0.79})
	plan := model.DivePlan{ID: 1, PlanCode: "SCORE-1", WorksitePressureBar: 1, BreathingMixJSON: mix, PlanStatus: constants.PlanDraft, Version: 1}
	diver := model.DiverProfile{ID: 1, ProfileCode: "SCORE-D", DefaultO2Fraction: 0.21, DefaultHeFraction: 0}
	segments := []model.ExposureSegment{
		{ID: 1, PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 2, GasMixJSON: mix, SegmentType: "descent"},
		{ID: 2, PlanID: 1, SequenceNo: 2, DepthM: 20, DurationMin: 18, GasMixJSON: mix, SegmentType: "bottom"},
		{ID: 3, PlanID: 1, SequenceNo: 3, DepthM: 0, DurationMin: 2, AscentRateMMin: 10, GasMixJSON: mix, SegmentType: "ascent"},
	}
	return plan, diver, segments
}

func TestCurvesNoDuplicateEmpty(t *testing.T) {
	plan, diver, segments := scoringPlan003()
	result, err := Run(plan, diver, segments, "training-v1", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Curves) != 6 {
		t.Fatalf("curves=%d, want 6", len(result.Curves))
	}
	for i, curve := range result.Curves {
		if len(curve.Points) == 0 {
			t.Fatalf("curve %d has no points", i)
		}
	}
}

func TestCurvePointsNoDuplicateZero(t *testing.T) {
	plan, diver, segments := scoringPlan003()
	result, err := Run(plan, diver, segments, "training-v1", 24)
	if err != nil {
		t.Fatal(err)
	}
	want := len(segments) + 1
	for _, curve := range result.Curves {
		if len(curve.Points) != want {
			t.Fatalf("curve %s points=%d, want %d", curve.Name, len(curve.Points), want)
		}
		for idx, point := range curve.Points {
			if point.SequenceNo == 0 && idx != 0 {
				t.Fatalf("curve %s has a zero-sequence point at index %d", curve.Name, idx)
			}
		}
	}
}

func TestRiskRelativeChangeAllCompartments(t *testing.T) {
	plan, _, _ := scoringPlan003()
	curves := []CompartmentCurve{
		{Name: "C05", Points: []LoadPoint{{SequenceNo: 0, RelativeChange: 0.4}}},
		{Name: "C120", Points: []LoadPoint{{SequenceNo: 0, RelativeChange: 2.6}}},
	}
	segments := []model.ExposureSegment{
		{ID: 1, PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 5, GasMixJSON: plan.BreathingMixJSON, SegmentType: "bottom"},
	}
	flags := EvaluateRisk(plan, segments, curves)
	for _, flag := range flags {
		if flag.Code == "RELATIVE_LOAD_CHANGE_ELEVATED" {
			return
		}
	}
	t.Fatalf("relative load elevated flag missing: %+v", flags)
}

func TestTransitionRiskAllSegments(t *testing.T) {
	mix, _ := EncodeGasMix(GasMix{O2: 0.21, He: 0, N2: 0.79})
	plan := model.DivePlan{ID: 1, PlanCode: "SCORE-2", WorksitePressureBar: 1, BreathingMixJSON: mix, PlanStatus: constants.PlanDraft, Version: 1}
	diver := model.DiverProfile{ID: 1, ProfileCode: "SCORE-D2", DefaultO2Fraction: 0.21, DefaultHeFraction: 0}
	segments := []model.ExposureSegment{
		{ID: 1, PlanID: 1, SequenceNo: 1, DepthM: 15, DurationMin: 3, GasMixJSON: mix, SegmentType: "descent"},
		{ID: 2, PlanID: 1, SequenceNo: 2, DepthM: 15, DurationMin: 5, GasMixJSON: mix, SegmentType: "bottom"},
		{ID: 3, PlanID: 1, SequenceNo: 3, DepthM: 0, DurationMin: 1, AscentRateMMin: 15, GasMixJSON: mix, SegmentType: "ascent"},
	}
	result, err := Run(plan, diver, segments, "training-v1", 24)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range result.RiskFlags {
		if flag.Code == "TRANSITION_RATE_CAUTION" {
			return
		}
	}
	t.Fatalf("transition rate caution flag missing: %+v", result.RiskFlags)
}

func TestDurationRiskAllSegments(t *testing.T) {
	mix, _ := EncodeGasMix(GasMix{O2: 0.21, He: 0, N2: 0.79})
	plan := model.DivePlan{ID: 1, PlanCode: "SCORE-3", WorksitePressureBar: 1, BreathingMixJSON: mix, PlanStatus: constants.PlanDraft, Version: 1}
	diver := model.DiverProfile{ID: 1, ProfileCode: "SCORE-D3", DefaultO2Fraction: 0.21, DefaultHeFraction: 0}
	segments := []model.ExposureSegment{
		{ID: 1, PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 2, GasMixJSON: mix, SegmentType: "descent"},
		{ID: 2, PlanID: 1, SequenceNo: 2, DepthM: 20, DurationMin: 95, GasMixJSON: mix, SegmentType: "bottom"},
		{ID: 3, PlanID: 1, SequenceNo: 3, DepthM: 0, DurationMin: 2, AscentRateMMin: 10, GasMixJSON: mix, SegmentType: "ascent"},
	}
	result, err := Run(plan, diver, segments, "training-v1", 24)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range result.RiskFlags {
		if flag.Code == "DURATION_ASSUMPTION_CAUTION" {
			return
		}
	}
	t.Fatalf("duration caution flag missing: %+v", result.RiskFlags)
}
