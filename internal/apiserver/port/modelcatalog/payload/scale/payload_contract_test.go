package scale_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	oldscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	newscale "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestScalePayloadJSONShapeMatchesLegacySnapshot(t *testing.T) {
	maxScore := 10.0
	oldPayload := &oldscale.ScaleSnapshot{
		ID:                   42,
		Code:                 "SCL_CONTRACT",
		ScaleVersion:         "1.0.0",
		Title:                "Contract Scale",
		QuestionnaireCode:    "Q_CONTRACT",
		QuestionnaireVersion: "2.0.0",
		Status:               "published",
		Factors: []oldscale.FactorSnapshot{{
			Code:            "total",
			Title:           "Total",
			IsTotalScore:    true,
			QuestionCodes:   []string{"Q1", "Q2"},
			ScoringStrategy: "sum",
			ScoringParams:   oldscale.ScoringParamsSnapshot{CntOptionContents: []string{"yes"}},
			MaxScore:        &maxScore,
			InterpretRules: []oldscale.InterpretRuleSnapshot{{
				Min:        0,
				Max:        10,
				RiskLevel:  "low",
				Conclusion: "low",
				Suggestion: "watch",
			}},
		}},
	}
	newPayload := &newscale.ScaleSnapshot{
		ID:                   42,
		Code:                 "SCL_CONTRACT",
		ScaleVersion:         "1.0.0",
		Title:                "Contract Scale",
		QuestionnaireCode:    "Q_CONTRACT",
		QuestionnaireVersion: "2.0.0",
		Status:               "published",
		Factors: []newscale.FactorSnapshot{{
			Code:            "total",
			Title:           "Total",
			IsTotalScore:    true,
			QuestionCodes:   []string{"Q1", "Q2"},
			ScoringStrategy: "sum",
			ScoringParams:   newscale.ScoringParamsSnapshot{CntOptionContents: []string{"yes"}},
			MaxScore:        &maxScore,
			InterpretRules: []newscale.InterpretRuleSnapshot{{
				Min:        0,
				Max:        10,
				RiskLevel:  "low",
				Conclusion: "low",
				Suggestion: "watch",
			}},
		}},
	}

	oldBytes, err := json.Marshal(oldPayload)
	if err != nil {
		t.Fatalf("marshal legacy scale payload: %v", err)
	}
	newBytes, err := json.Marshal(newPayload)
	if err != nil {
		t.Fatalf("marshal new scale payload: %v", err)
	}
	if !bytes.Equal(newBytes, oldBytes) {
		t.Fatalf("new payload JSON = %s\nlegacy JSON = %s", newBytes, oldBytes)
	}

	decoded, err := newscale.ParsePublishedPayload(newBytes)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if !reflect.DeepEqual(decoded, oldPayload) {
		t.Fatalf("decoded = %#v, want %#v", decoded, oldPayload)
	}
}

func TestScalePayloadDefinitionRoundTripMatchesLegacySnapshot(t *testing.T) {
	maxScore := 10.0
	original := &newscale.ScaleSnapshot{
		Code:         "SCL_DEF",
		ScaleVersion: "1.0.0",
		Title:        "Definition Scale",
		Status:       "published",
		Factors: []newscale.FactorSnapshot{{
			Code:            "total",
			Title:           "Total",
			IsTotalScore:    true,
			QuestionCodes:   []string{"Q1"},
			ScoringStrategy: "sum",
			MaxScore:        &maxScore,
			InterpretRules: []newscale.InterpretRuleSnapshot{{
				Min:        0,
				Max:        10,
				RiskLevel:  "low",
				Conclusion: "low",
				Suggestion: "watch",
			}},
		}},
	}

	definition := newscale.DefinitionFromScaleSnapshot(original)
	got := newscale.ScaleSnapshotFromDefinition(newscale.ExecutionEnvelope{
		Code:         original.Code,
		ScaleVersion: original.ScaleVersion,
		Title:        original.Title,
		Status:       original.Status,
	}, definition)
	if got == nil || len(got.Factors) != 1 {
		t.Fatalf("round trip snapshot = %#v", got)
	}
	if got.Factors[0].Code != "total" || !got.Factors[0].IsTotalScore || got.Factors[0].MaxScore == nil {
		t.Fatalf("factor round trip = %#v", got.Factors[0])
	}
	if got.Factors[0].Canonical().ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("canonical role = %q, want total", got.Factors[0].Canonical().ResolvedRole())
	}
}
