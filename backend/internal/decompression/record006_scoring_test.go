package decompression

import (
	"testing"

	"commercial-diving-decompression-control/backend/internal/model"
)

func TestDecodeGasMixErrorNotZero(t *testing.T) {
	mix, err := DecodeGasMix(`{"o2":0.21,"n2":0.5}`)
	if err == nil {
		t.Fatalf("DecodeGasMix swallowed invalid gas and returned zero value %+v", mix)
	}
}

func TestEncodeGasMixErrorNotZero(t *testing.T) {
	encoded, err := EncodeGasMix(GasMix{O2: 0.21, He: 0, N2: 0.7})
	if err == nil {
		t.Fatalf("EncodeGasMix swallowed invalid gas and returned %q", encoded)
	}
}

func TestCalculateCurvesGasErrorNotZero(t *testing.T) {
	plan := model.DivePlan{ID: 1, WorksitePressureBar: 1, BreathingMixJSON: `{"o2":0.21,"he":0,"n2":0.79}`}
	diver := model.DiverProfile{ID: 1, DefaultO2Fraction: 0.21, DefaultHeFraction: 0}
	segments := []model.ExposureSegment{
		{ID: 1, PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 10, GasMixJSON: `{"o2":0.05,"n2":0.95}`, SegmentType: "bottom"},
	}
	_, err := calculateCurves(plan, diver, segments, DefaultCompartments())
	if err == nil {
		t.Fatal("calculateCurves swallowed invalid segment gas")
	}
}
