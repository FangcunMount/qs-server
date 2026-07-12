package outcome

import (
	"strings"
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func TestExecutionForRecordV2ExcludesReportProse(t *testing.T) {
	execution := &domainoutcome.Execution{
		Summary: domainoutcome.Summary{PrimaryLabel: "INTJ", Tags: []string{"建筑师", "独立战略家"}},
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
	for _, forbidden := range []string{"one_liner", "summary\"", "strengths", "weaknesses", "suggestions", "commentary", "image_url", "rarity", "independent strategist"} {
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
}
