package outcome

import (
	"encoding/json"
	"testing"
	"time"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestRestoreUsesOnlyPersistedOutcomeAndRestoresTypedDetail(t *testing.T) {
	detail := outcometypology.PersonalityTypeDetail{TypeCode: "INTJ", TypeName: "Architect"}
	execution := assessment.NewAssessmentOutcome(
		assessment.NewEvaluationModelRefWithIdentity(assessment.EvaluationModelKindTypology, modelcatalog.SubKindTypology, modelcatalog.AlgorithmMBTI, 0, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI"),
		assessment.ResultSummary{PrimaryLabel: "INTJ"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindTypology, Payload: detail},
	)
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	reportInput, err := json.Marshal(evaluationinput.TypologyModelPayload{Payload: &modeltypology.Payload{Code: "MBTI-16P", Version: "1.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID: meta.FromUint64(9), OrgID: 11, AssessmentID: meta.FromUint64(7), TesteeID: 8, RunID: "7:1",
		Model:   domainoutcome.ModelIdentity{Kind: modelcatalog.KindTypology, SubKind: modelcatalog.SubKindTypology, Algorithm: modelcatalog.AlgorithmMBTI, Code: "MBTI-16P", Version: "1.0.0", Title: "MBTI"},
		Runtime: domainoutcome.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		Payload: payload, ReportInput: reportInput, EvaluatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := Restore(record)
	if err != nil {
		t.Fatal(err)
	}
	if got.Assessment == nil || got.Assessment.ID().Uint64() != 7 || got.Assessment.TesteeID().Uint64() != 8 || !got.Assessment.Status().IsEvaluated() {
		t.Fatalf("restored assessment context = %#v", got.Assessment)
	}
	if typed, ok := got.Execution.Detail.Payload.(outcometypology.PersonalityTypeDetail); !ok || typed.TypeCode != "INTJ" {
		t.Fatalf("restored detail = %#v (%T)", got.Execution.Detail.Payload, got.Execution.Detail.Payload)
	}
	if restored, ok := evaluationinput.TypologyPayload(got.Input); !ok || restored.Code != "MBTI-16P" {
		t.Fatalf("restored report input = %#v", got.Input)
	}
}
