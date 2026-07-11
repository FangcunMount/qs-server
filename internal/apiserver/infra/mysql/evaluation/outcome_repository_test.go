package evaluation

import (
	"testing"
	"time"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvaluationOutcomePersistenceMappingRoundTrip(t *testing.T) {
	t.Parallel()

	evaluatedAt := time.Unix(123, 456000000)
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           meta.FromUint64(7001),
		OrgID:        11,
		AssessmentID: meta.FromUint64(5001),
		TesteeID:     3001,
		RunID:        "5001:1",
		Model: domainoutcome.ModelIdentity{
			Kind:      modelcatalog.KindTypology,
			SubKind:   modelcatalog.SubKindTypology,
			Algorithm: modelcatalog.AlgorithmMBTI,
			Code:      "MBTI-16P",
			Version:   "1.0.0",
			Title:     "MBTI",
		},
		Runtime: domainoutcome.RuntimeIdentity{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindPoleComposition,
			PayloadFormat:   "typology.v2",
		},
		InputSnapshotRef: "model:MBTI-16P@1.0.0",
		ReportInput:      []byte(`{"Payload":{"code":"MBTI-16P"}}`),
		Payload:          []byte(`{"summary":{"PrimaryLabel":"INTJ"}}`),
		SchemaVersion:    1,
		EvaluatedAt:      evaluatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	po := outcomeToPO(record)
	got, err := outcomeFromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != record.ID() || got.OrgID() != record.OrgID() || got.AssessmentID() != record.AssessmentID() || got.TesteeID() != record.TesteeID() || got.RunID() != record.RunID() {
		t.Fatalf("identity round trip: %#v", got)
	}
	if got.Model() != record.Model() || got.Runtime() != record.Runtime() {
		t.Fatalf("routing round trip: model=%#v runtime=%#v", got.Model(), got.Runtime())
	}
	if string(got.Payload()) != string(record.Payload()) || !got.EvaluatedAt().Equal(evaluatedAt) {
		t.Fatalf("payload/time round trip: payload=%s evaluated_at=%s", got.Payload(), got.EvaluatedAt())
	}
	if string(got.ReportInput()) != string(record.ReportInput()) {
		t.Fatalf("report input = %s", got.ReportInput())
	}
}
