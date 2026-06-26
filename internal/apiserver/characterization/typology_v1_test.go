package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	bigfiveadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/bigfive"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// V1 contract: typology executor scores legacy MBTI payload identically to domain scorer.
func TestV1TypologyMBTIExecutorPreservesLegacyScoringOutcome(t *testing.T) {
	model := mbtiINTJModel()
	want, err := evaluationtypology.ScoreMBTIReference(model, mbtiINTJAnswerSheet())
	if err != nil {
		t.Fatalf("domain Score: %v", err)
	}

	executor, err := typologyeval.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
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

	executor, err := typologyeval.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
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

// V1 contract: typology executor scores v2 Big Five payload through trait-profile pipeline.
func TestV1TypologyBigFiveExecutorPreservesTraitProfileOutcome(t *testing.T) {
	model := bigFiveCharacterizationModel()
	want, err := bigfiveadapter.Score(model, bigFiveAnswerSheet())
	if err != nil {
		t.Fatalf("domain Score: %v", err)
	}

	executor, err := typologyeval.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: submittedBigFiveAssessment(t),
		Input:      bigFiveInputSnapshot(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail, ok := result.Detail.Payload.(evaluationtypology.BigFiveResultDetail)
	if !ok {
		t.Fatalf("payload type = %T, want typology.BigFiveResultDetail", result.Detail.Payload)
	}
	if len(detail.Traits) != len(want.Traits) || detail.Traits[0].RawScore != want.Traits[0].RawScore {
		t.Fatalf("detail = %#v, want traits %#v", detail.Traits, want.Traits)
	}
	if result.Summary.PrimaryLabel != "O" {
		t.Fatalf("PrimaryLabel = %q, want O", result.Summary.PrimaryLabel)
	}
	if result.Profile == nil || result.Profile.Kind != assessment.ProfileKindPersonalityTrait {
		t.Fatalf("profile = %#v, want personality_trait", result.Profile)
	}
}

// V1 contract: configured typology executor advertises the generic routing key.
func TestV1ConfiguredTypologyExecutorKey(t *testing.T) {
	executor, err := typologyeval.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	if got := executor.Key(); got != evaluation.EvaluatorKeyPersonalityTypology {
		t.Fatalf("executor key = %s, want %s", got, evaluation.EvaluatorKeyPersonalityTypology)
	}
}
