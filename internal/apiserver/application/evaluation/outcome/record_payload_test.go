package outcome

import (
	"encoding/json"
	"strings"
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func TestExecutionForRecordV2ExcludesReportProse(t *testing.T) {
	execution := &domainoutcome.Execution{
		ModelRef: domainoutcome.ModelRef{ModelTitle: "display model"},
		Summary:  domainoutcome.Summary{PrimaryLabel: "INTJ", Tags: []string{"建筑师", "独立战略家"}},
		Primary:  &domainoutcome.ScoreValue{Value: 80, Label: "display score"},
		Level:    &domainoutcome.ResultLevel{Code: "high", Label: "display level"},
		Profile:  &domainoutcome.ProfileResult{Code: "INTJ", Name: "display profile"},
		Dimensions: []domainoutcome.DimensionResult{{
			Code: "EI", Name: "display dimension", Score: &domainoutcome.ScoreValue{Value: 1, Label: "display dimension score"},
			DerivedScores: []domainoutcome.ScoreValue{{Kind: domainoutcome.ScoreKindTScore, Value: 65}},
			Level:         &domainoutcome.ResultLevel{Code: "high", Label: "display dimension level"}, NormReference: &domainoutcome.NormReference{ScoreKind: domainoutcome.ScoreKindTScore, Benchmark: 50, TableVersion: "2026", MinAgeMonths: 60, MaxAgeMonths: 95}, LeftPole: "left display", RightPole: "right display", Model: "display dimension model",
		}},
		Validity: []domainoutcome.ValidityResult{{Code: "valid", Label: "display validity", Passed: true, Message: "display validity message"}},
		Detail: domainoutcome.Detail{Payload: outcometypology.PersonalityTypeDetail{
			TypeCode: "INTJ", TypeName: "建筑师", OneLiner: "独立战略家", Summary: "summary",
			Strengths: []string{"strength"}, Weaknesses: []string{"weakness"}, Suggestions: []string{"suggestion"}, Commentary: "commentary",
			ImageURL: "image", MatchPercent: 80,
		}},
	}
	payload, err := MarshalRecordV2(execution)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(payload))
	for _, forbidden := range []string{"one_liner", "summary\"", "strengths", "weaknesses", "suggestions", "commentary", "image_url", "rarity", "independent strategist", "display model", "display score", "display level", "display profile", "display dimension", "left display", "right display", "display validity"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("schema v2 payload contains report field %q: %s", forbidden, payload)
		}
	}
	if !strings.Contains(string(payload), "INTJ") || !strings.Contains(lower, "match_percent") {
		t.Fatalf("schema v2 payload lost classification facts: %s", payload)
	}
	if len(execution.Summary.Tags) != 2 {
		t.Fatal("source execution was mutated")
	}
	var durable struct {
		Dimensions []domainoutcome.DimensionResult
	}
	if err := json.Unmarshal(payload, &durable); err != nil {
		t.Fatal(err)
	}
	if len(durable.Dimensions) != 1 || len(durable.Dimensions[0].DerivedScores) != 1 || durable.Dimensions[0].NormReference == nil || durable.Dimensions[0].NormReference.TableVersion != "2026" || durable.Dimensions[0].NormReference.Benchmark != 50 {
		t.Fatalf("durable norm facts = %#v", durable.Dimensions)
	}
}
