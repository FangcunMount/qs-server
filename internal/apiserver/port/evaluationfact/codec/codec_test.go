package codec

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestDecodeExecutionConvertsLegacyScaleDetail(t *testing.T) {
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(10), AssessmentID: meta.FromUint64(8), TesteeID: 9, RunID: "8:1",
		Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindScale, Code: "SDS", Version: "1.0.0"},
		Runtime:       evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring},
		SchemaVersion: 1, EvaluatedAt: time.Unix(100, 0),
		Payload: []byte(`{"ModelRef":{"kind":"scale","code":"SDS"},"Detail":{"Kind":"scale","Payload":[{"FactorCode":"total","FactorName":"总分","RawScore":42,"RiskLevel":"medium","Conclusion":"legacy prose","Suggestion":"legacy prose","IsTotalScore":true}]}}`),
	})
	execution, err := DecodeExecution(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Dimensions) != 1 || execution.Dimensions[0].Role != "total" || execution.Dimensions[0].Score.Value != 42 {
		t.Fatalf("dimensions = %#v", execution.Dimensions)
	}
	if execution.Detail.Payload != nil {
		t.Fatalf("legacy detail escaped codec: %#v", execution.Detail.Payload)
	}
}

func TestDecodeExecutionRejectsUnknownSchema(t *testing.T) {
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: meta.FromUint64(1), SchemaVersion: 99, Payload: []byte(`{}`)})
	if _, err := DecodeExecution(record); err == nil {
		t.Fatal("expected unsupported schema error")
	}
}

func TestDecodeExecutionRetainsLegacyMBTIProfile(t *testing.T) {
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(11), SchemaVersion: 1,
		Model:   evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmMBTI, Code: "MBTI"},
		Runtime: evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","type_name":"建筑师","match_percent":40,"profile":{"summary":"独立战略家","strengths":["系统思考"],"suggestions":["保留沟通空间"]},"source":{"attribution":"OEJTS"}}}}`),
	})
	execution, err := DecodeExecution(record)
	if err != nil {
		t.Fatal(err)
	}
	detail, ok := PersonalityTypeDetailFromPayload(execution.Detail.Payload)
	if !ok || detail.Summary != "独立战略家" || len(detail.Strengths) != 1 || len(detail.Suggestions) != 1 {
		t.Fatalf("legacy MBTI detail = %#v", execution.Detail.Payload)
	}
}
