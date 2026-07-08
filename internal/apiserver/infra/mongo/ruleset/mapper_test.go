package ruleset

import (
	v1envelope "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/v1envelope"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestMapperRoundTrip(t *testing.T) {
	snapshot := &v1envelope.V1Snapshot{
		Definition: v1envelope.V1Definition{
			Kind:    v1envelope.RuleSetKindMBTI,
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
			Title:   "MBTI",
			Status:  "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
		},
		DecisionKind: domain.DecisionKindPoleComposition,
		Source: map[string]any{
			"license": "CC BY-NC-SA 4.0",
		},
		Payload: []byte(`{"code":"MBTI_OEJTS"}`),
	}

	mapper := NewMapper()
	po := mapper.ToPO(snapshot)
	got := mapper.ToDomain(po)
	if got.Definition.Code != snapshot.Definition.Code {
		t.Fatalf("code = %s, want %s", got.Definition.Code, snapshot.Definition.Code)
	}
	if string(got.Payload) != string(snapshot.Payload) {
		t.Fatalf("payload = %s, want %s", got.Payload, snapshot.Payload)
	}
}
