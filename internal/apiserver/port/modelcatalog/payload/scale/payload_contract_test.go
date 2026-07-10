package scale_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	newscale "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestScalePayloadJSONShapeMatchesLegacySnapshot(t *testing.T) {
	maxScore := 10.0
	payload := &newscale.ScaleSnapshot{
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

	gotBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal new scale payload: %v", err)
	}
	wantBytes := []byte(`{"ID":42,"Code":"SCL_CONTRACT","ScaleVersion":"1.0.0","Title":"Contract Scale","QuestionnaireCode":"Q_CONTRACT","QuestionnaireVersion":"2.0.0","Status":"published","Factors":[{"Code":"total","Title":"Total","IsTotalScore":true,"QuestionCodes":["Q1","Q2"],"ScoringStrategy":"sum","ScoringParams":{"CntOptionContents":["yes"]},"MaxScore":10,"InterpretRules":[{"Min":0,"Max":10,"RiskLevel":"low","Conclusion":"low","Suggestion":"watch"}]}]}`)
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("payload JSON = %s\nwant JSON = %s", gotBytes, wantBytes)
	}

	decoded, err := newscale.ParsePublishedPayload(gotBytes)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if !reflect.DeepEqual(decoded, payload) {
		t.Fatalf("decoded = %#v, want %#v", decoded, payload)
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
	visible, configured := definition.ReportMap.FactorScoreSources()
	if !configured || len(visible) != 1 || visible[0] != "total" {
		t.Fatalf("factor score report map = (%#v, %v)", visible, configured)
	}
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
	if !got.Factors[0].IsTotalScore {
		t.Fatalf("factor IsTotalScore = false, want true")
	}
}
