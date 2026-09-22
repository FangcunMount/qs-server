package input

import (
	"math"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func mbtiPoleFactsFixture() ([]patterns.PersonalityTypeDimensionReport, *evaluationfact.Execution, *evaluationinput.InputSnapshot) {
	codes := []string{"EI", "SN", "TF", "JP"}
	left := []string{"I", "S", "F", "J"}
	right := []string{"E", "N", "T", "P"}
	raw := []float64{15, 23, 20, 12}
	strength := []float64{56.25, 6.25, 25, 75}
	dims := make([]patterns.PersonalityTypeDimensionReport, 4)
	execution := &evaluationfact.Execution{}
	assets := &evaluationinput.InputSnapshot{MBTIPoles: &evaluationinput.MBTIPoleCatalog{SchemaVersion: evaluationinput.MBTIPoleCatalogSchema}}
	for i, c := range codes {
		dims[i] = patterns.PersonalityTypeDimensionReport{Code: c, RawScore: raw[i], Preference: left[i], Strength: strength[i]}
		execution.Dimensions = append(execution.Dimensions, evaluationfact.DimensionResult{Code: c, Score: &evaluationfact.ScoreValue{Kind: evaluationfact.ScoreKindRawTotal, Value: raw[i]}, Preference: left[i], Strength: &strength[i]})
		assets.MBTIPoles.Axes = append(assets.MBTIPoles.Axes, evaluationinput.MBTIPoleAxis{Code: c, Name: c + " axis", LeftPole: left[i], RightPole: right[i], MinScore: 8, MaxScore: 40, Threshold: 24})
	}
	return dims, execution, assets
}

func TestMBTIPolesPreservePreciseOutcomeAndFrozenAxes(t *testing.T) {
	dims, e, a := mbtiPoleFactsFixture()
	if err := attachFrozenMBTIPoles(dims, e, a, "ISFJ"); err != nil {
		t.Fatal(err)
	}
	if dims[0].PoleFacts.Strength != 56.25 || dims[1].PoleFacts.Strength != 6.25 || dims[0].Name != "EI axis" || dims[0].PoleFacts.LeftPole != "I" {
		t.Fatal("facts lost")
	}
	// Known zero remains distinguishable from missing strength.
	zero := 0.0
	e.Dimensions[0].Strength = &zero
	e.Dimensions[0].Score.Value = 24
	if err := attachFrozenMBTIPoles(dims, e, a, "ISFJ"); err != nil {
		t.Fatal(err)
	}
	if dims[0].PoleFacts.Strength != 0 {
		t.Fatal("known zero lost")
	}
}

func TestMBTIPolesRejectIncompleteOrConflictingOutcome(t *testing.T) {
	tests := map[string]func(*evaluationfact.Execution){
		"missing strength":   func(e *evaluationfact.Execution) { e.Dimensions[0].Strength = nil },
		"wrong score unit":   func(e *evaluationfact.Execution) { e.Dimensions[0].Score.Kind = evaluationfact.ScoreKindMatchPercent },
		"missing score":      func(e *evaluationfact.Execution) { e.Dimensions[0].Score = nil },
		"duplicate":          func(e *evaluationfact.Execution) { e.Dimensions[1].Code = "EI" },
		"unknown axis":       func(e *evaluationfact.Execution) { e.Dimensions[0].Code = "OTHER" },
		"wrong pole":         func(e *evaluationfact.Execution) { e.Dimensions[0].LeftPole = "E" },
		"unknown preference": func(e *evaluationfact.Execution) { e.Dimensions[0].Preference = "X" },
		"type conflict":      func(e *evaluationfact.Execution) { e.Dimensions[0].Preference = "E" },
		"out of range":       func(e *evaluationfact.Execution) { e.Dimensions[0].Score.Value = 41 },
		"nonfinite":          func(e *evaluationfact.Execution) { n := math.NaN(); e.Dimensions[0].Strength = &n },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			dims, e, a := mbtiPoleFactsFixture()
			change(e)
			if err := attachFrozenMBTIPoles(dims, e, a, "ISFJ"); err == nil {
				t.Fatal("invalid outcome accepted")
			}
		})
	}
}

func TestLegacyMBTIPolesRemainAbsent(t *testing.T) {
	dims, e, a := mbtiPoleFactsFixture()
	a.MBTIPoles = nil
	if err := attachFrozenMBTIPoles(dims, e, a, "ISFJ"); err != nil || dims[0].PoleFacts != nil {
		t.Fatal("legacy facts synthesized", err)
	}
}
