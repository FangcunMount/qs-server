package characterization_test

import (
	"context"
	"testing"

	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
)

// V1 contract: typology executor scores legacy MBTI payload identically to domain scorer.
func TestV1TypologyMBTIExecutorPreservesLegacyScoringOutcome(t *testing.T) {
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
	detail := requirePersonalityTypeDetail(t, result.Detail.Payload)
	if detail.TypeCode != "INTJ" || detail.MatchPercent != 40 {
		t.Fatalf("detail = %#v, want type=INTJ match=40", detail)
	}
	if result.Summary.PrimaryLabel != "INTJ" {
		t.Fatalf("PrimaryLabel = %q, want INTJ", result.Summary.PrimaryLabel)
	}
}

// V1 contract: typology executor scores legacy SBTI payload identically to domain scorer.
func TestV1TypologySBTIExecutorPreservesLegacyScoringOutcome(t *testing.T) {
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
	detail := requirePersonalityTypeDetail(t, result.Detail.Payload)
	if detail.TypeCode != "HIGH" || detail.Similarity != 1 {
		t.Fatalf("detail = %#v, want type=HIGH similarity=1", detail)
	}
	if result.Summary.Score == nil || *result.Summary.Score != 100 {
		t.Fatalf("Score = %v, want 100", result.Summary.Score)
	}
}

// V1 contract: typology executor scores v2 Big Five payload through trait-profile pipeline.
func TestV1TypologyBigFiveExecutorPreservesTraitProfileOutcome(t *testing.T) {
	model := bigFiveCharacterizationModel()
	want, err := scoreBigFiveCharacterization(t, model, bigFiveAnswerSheet())
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
	detail := requireTraitProfileDetail(t, result.Detail.Payload)
	if len(detail.Traits) != len(want.Traits) || detail.Traits[0].RawScore != want.Traits[0].RawScore {
		t.Fatalf("detail = %#v, want traits %#v", detail.Traits, want.Traits)
	}
	if result.Summary.PrimaryLabel != "O" {
		t.Fatalf("PrimaryLabel = %q, want O", result.Summary.PrimaryLabel)
	}
	if result.Profile == nil || result.Profile.Kind != domainoutcome.ProfileKindPersonalityTrait {
		t.Fatalf("profile = %#v, want personality_trait", result.Profile)
	}
}

// V1 contract: configured typology executor advertises the generic routing key.
func TestV1ConfiguredTypologyExecutorKey(t *testing.T) {
	executor, err := typologyeval.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	if got := executor.Key(); got != evaluation.ExecutionIdentityPersonalityTypology {
		t.Fatalf("executor key = %s, want %s", got, evaluation.ExecutionIdentityPersonalityTypology)
	}
}
