package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvidenceServiceUsesAggregateVersionCAS(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	repository := &evidenceRepositoryStub{}
	service, err := NewEvidenceService(repository, func() meta.ID { return meta.ID(501) }, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	runRecord, err := service.Start(context.Background(), evidenceRelease())
	if err != nil {
		t.Fatal(err)
	}
	if runRecord.ID() != meta.ID(501) || repository.created != runRecord {
		t.Fatalf("created run = %#v", runRecord)
	}

	now = now.Add(time.Minute)
	runRecord, err = service.RecordAttempt(context.Background(), runRecord.ID(), domainevaluation.AttemptRecord{
		CaseID: "preflight", Attempt: 1, Stage: domainevaluation.AttemptStagePreflight,
		StartedAt: now, FinishedAt: now.Add(time.Second), ProviderCallCount: 0, RejectionReason: "insufficient_eligible_dimensions",
		Assertions: []domainevaluation.AssertionReceipt{{Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.expectedVersion != 1 || runRecord.Version() != 2 {
		t.Fatalf("expected/stored version = %d/%d", repository.expectedVersion, runRecord.Version())
	}

	repository.saveErr = domainevaluation.ErrConflict
	_, err = service.RecordAttempt(context.Background(), runRecord.ID(), domainevaluation.AttemptRecord{})
	if err == nil {
		t.Fatal("invalid attempt must fail before persistence")
	}
	if errors.Is(err, domainevaluation.ErrConflict) {
		t.Fatal("invalid aggregate mutation must not reach repository CAS")
	}
}

func evidenceRelease() domainevaluation.ReleaseIdentity {
	return domainevaluation.ReleaseIdentity{
		Suite:             domainevaluation.SuiteRef{ID: SuiteIDV1, Version: SuiteVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("suite")), GitBlobSHA: "suite-blob"},
		Prompt:            aiexplanation.PromptRef{TemplateID: "ai-explanation", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "prompt-blob"},
		Profile:           aiexplanation.ProfileRef{ID: "participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("profile"))},
		InputSchema:       domainevaluation.SchemaRef{Version: aiexplanation.InputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("input-schema"))},
		OutputSchema:      domainevaluation.SchemaRef{Version: aiexplanation.OutputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("output-schema"))},
		Provider:          aiexplanation.ProviderExecutionSpec{Route: "balanced_text_v1", RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("route"))},
		Decoding:          domainevaluation.DecodingParameters{MaxOutputTokens: 3000},
		SemanticEvaluator: evidenceSemanticEvaluator(),
		GenerationCaseIDs: []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"}, PreflightCaseID: "preflight",
		PreflightRejectionReason: "insufficient_eligible_dimensions", RepetitionsPerCase: 5,
	}
}

func evidenceSemanticEvaluator() domainevaluation.SemanticEvaluatorSpec {
	return domainevaluation.SemanticEvaluatorSpec{
		Version: "semantic-rubric-v1",
		Prompt: aiexplanation.PromptRef{
			TemplateID: "ai-explanation-semantic-evaluator", Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-prompt")), GitBlobSHA: "semantic-prompt-blob",
		},
		OutputSchema: domainevaluation.SchemaRef{Version: "ai-explanation-semantic-evaluation-output/v1", Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-schema"))},
		Provider: aiexplanation.ProviderExecutionSpec{
			Route: "semantic_judge_v1", RouteRevision: "v1", ResolvedProvider: "judge-provider", ResolvedModel: "judge-model",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-route")),
		},
		Decoding: domainevaluation.DecodingParameters{MaxOutputTokens: 2000},
	}
}

type evidenceRepositoryStub struct {
	created         *domainevaluation.PromptEvaluationRun
	expectedVersion int64
	saveErr         error
}

func (r *evidenceRepositoryStub) Create(_ context.Context, value *domainevaluation.PromptEvaluationRun) error {
	r.created = value
	return nil
}
func (r *evidenceRepositoryStub) Save(_ context.Context, value *domainevaluation.PromptEvaluationRun, expectedVersion int64) error {
	r.expectedVersion = expectedVersion
	if r.saveErr != nil {
		return r.saveErr
	}
	r.created = value
	return nil
}
func (r *evidenceRepositoryStub) FindByID(_ context.Context, id meta.ID) (*domainevaluation.PromptEvaluationRun, error) {
	if r.created == nil || r.created.ID() != id {
		return nil, domainevaluation.ErrNotFound
	}
	return r.created, nil
}
