package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// V1 contract: typology executor scores legacy MBTI payload identically to domain scorer.
func TestV1TypologyMBTIExecutorPreservesLegacyScoringOutcome(t *testing.T) {
	model := mbtiINTJModel()
	want, err := evaluationtypology.ScoreMBTIReference(model, mbtiINTJAnswerSheet())
	if err != nil {
		t.Fatalf("domain Score: %v", err)
	}

	executor, err := typologyeval.NewTypologyExecutor(assessmentmodel.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("NewTypologyExecutor: %v", err)
	}
	result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: submittedMBTIAssessment(t),
		Input:      mbtiInputSnapshot(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail, ok := result.Detail.Payload.(evaluationtypology.MBTIResultDetail)
	if !ok {
		t.Fatalf("payload type = %T, want typology.MBTIResultDetail", result.Detail.Payload)
	}
	if detail.TypeCode != want.TypeCode || detail.MatchPercent != want.MatchPercent {
		t.Fatalf("detail = %#v, want type=%s match=%.0f", detail, want.TypeCode, want.MatchPercent)
	}
	if result.Summary.PrimaryLabel != "INTJ" {
		t.Fatalf("PrimaryLabel = %q, want INTJ", result.Summary.PrimaryLabel)
	}
}

// V1 contract: typology executor scores legacy SBTI payload identically to domain scorer.
func TestV1TypologySBTIExecutorPreservesLegacyScoringOutcome(t *testing.T) {
	model := sbtiCharacterizationModel()
	want, err := evaluationtypology.ScoreSBTIReference(model, sbtiHighAnswerSheet())
	if err != nil {
		t.Fatalf("domain Score: %v", err)
	}

	executor, err := typologyeval.NewTypologyExecutor(assessmentmodel.AlgorithmSBTI)
	if err != nil {
		t.Fatalf("NewTypologyExecutor: %v", err)
	}
	result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: submittedSBTIAssessment(t),
		Input:      sbtiInputSnapshot(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail, ok := result.Detail.Payload.(evaluationtypology.SBTIResultDetail)
	if !ok {
		t.Fatalf("payload type = %T, want typology.SBTIResultDetail", result.Detail.Payload)
	}
	if detail.TypeCode != want.TypeCode || detail.Similarity != want.Similarity {
		t.Fatalf("detail = %#v, want type=%s similarity=%.0f", detail, want.TypeCode, want.Similarity)
	}
	if result.Summary.Score == nil || *result.Summary.Score != 100 {
		t.Fatalf("Score = %v, want 100", result.Summary.Score)
	}
}

// V1 contract: typology executor keys map to personality/typology/{mbti,sbti}.
func TestV1TypologyExecutorKeys(t *testing.T) {
	mbtiExecutor, err := typologyeval.NewTypologyExecutor(assessmentmodel.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("NewTypologyExecutor(mbti): %v", err)
	}
	if got := mbtiExecutor.Key().String(); got != "personality/typology/mbti" {
		t.Fatalf("mbti key = %q", got)
	}
	sbtiExecutor, err := typologyeval.NewTypologyExecutor(assessmentmodel.AlgorithmSBTI)
	if err != nil {
		t.Fatalf("NewTypologyExecutor(sbti): %v", err)
	}
	if got := sbtiExecutor.Key().String(); got != "personality/typology/sbti" {
		t.Fatalf("sbti key = %q", got)
	}
}
