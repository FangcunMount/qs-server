package governance

import (
	"context"
	"errors"
	"testing"
	"time"

	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestPublishBindsApprovedEvaluationEvidence(t *testing.T) {
	now := time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC)
	profileRecord := testDraftProfile(t, meta.ID(1001), now.Add(-time.Hour))
	evidence := testApprovedEvidence(t, meta.ID(2001), profileRecord, now.Add(-2*time.Hour))
	profiles := &profileRepositoryStub{records: map[string]*domainprofile.AIExplanationProfile{profileKey(profileRecord.ProfileID(), profileRecord.Version()): profileRecord}}
	evaluations := &evaluationRepositoryStub{records: map[meta.ID]*domainevaluation.PromptEvaluationRun{evidence.ID(): evidence}}
	service, err := NewService(profiles, evaluations, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	published, err := service.Publish(context.Background(), PublishCommand{
		ProfileID: profileRecord.ProfileID(), ProfileVersion: profileRecord.Version(), EvaluationRunID: evidence.ID(),
		Actor: "release-owner", Reason: "rubric and dual review passed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if published.Status() != domainprofile.StatusPublished || published.PublishedEvidenceRunID() != evidence.ID() || published.PublishedBy() != "release-owner" || published.PublishedReason() != "rubric and dual review passed" {
		t.Fatalf("published Profile audit = %#v", published)
	}
	if profiles.saveCalls != 1 {
		t.Fatalf("Profile save calls = %d", profiles.saveCalls)
	}

	disabled, err := service.Disable(context.Background(), DisableCommand{
		ProfileID: profileRecord.ProfileID(), ProfileVersion: profileRecord.Version(), Actor: "release-owner", Reason: "route retired",
	})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Status() != domainprofile.StatusDisabled || disabled.DisabledReason() != "route retired" || disabled.PublishedEvidenceRunID() != evidence.ID() {
		t.Fatalf("disabled Profile audit = %#v", disabled)
	}
}

func TestCreateDraftRecomputesFingerprintAndPersistsCreationAudit(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	suite, err := appevaluation.LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := suite.ProfileFixture.Definition.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	profiles := &profileRepositoryStub{records: map[string]*domainprofile.AIExplanationProfile{}}
	service, err := NewService(profiles, &evaluationRepositoryStub{records: map[meta.ID]*domainevaluation.PromptEvaluationRun{}}, func() time.Time { return now }, func() meta.ID { return meta.ID(3001) })
	if err != nil {
		t.Fatal(err)
	}
	draft, err := service.CreateDraft(context.Background(), CreateDraftCommand{
		Definition: suite.ProfileFixture.Definition, ExpectedFingerprint: fingerprint,
		Actor: "user:42", Reason: "initial release candidate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID() != meta.ID(3001) || draft.Status() != domainprofile.StatusDraft || draft.CreatedBy() != "user:42" || draft.CreatedReason() != "initial release candidate" || profiles.saveCalls != 1 {
		t.Fatalf("draft = %#v, save calls = %d", draft, profiles.saveCalls)
	}

	_, err = service.CreateDraft(context.Background(), CreateDraftCommand{
		Definition: suite.ProfileFixture.Definition, ExpectedFingerprint: aiexplanation.NewFingerprint([]byte("wrong")),
		Actor: "user:42", Reason: "wrong fingerprint",
	})
	if !errors.Is(err, ErrProfileFingerprint) || profiles.saveCalls != 1 {
		t.Fatalf("fingerprint mismatch = %v, save calls = %d", err, profiles.saveCalls)
	}
}

func TestPublishRejectsIncompleteOrMismatchedEvidence(t *testing.T) {
	now := time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC)
	profileRecord := testDraftProfile(t, meta.ID(1002), now.Add(-time.Hour))
	release := testRelease(profileRecord)
	incomplete, err := domainevaluation.New(meta.ID(2002), release, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	profiles := &profileRepositoryStub{records: map[string]*domainprofile.AIExplanationProfile{profileKey(profileRecord.ProfileID(), profileRecord.Version()): profileRecord}}
	evaluations := &evaluationRepositoryStub{records: map[meta.ID]*domainevaluation.PromptEvaluationRun{incomplete.ID(): incomplete}}
	service, _ := NewService(profiles, evaluations, func() time.Time { return now })
	_, err = service.Publish(context.Background(), PublishCommand{
		ProfileID: profileRecord.ProfileID(), ProfileVersion: profileRecord.Version(), EvaluationRunID: incomplete.ID(), Actor: "owner", Reason: "try",
	})
	if !errors.Is(err, ErrPublishEvidenceRequired) || profiles.saveCalls != 0 {
		t.Fatalf("incomplete evidence error/save calls = %v/%d", err, profiles.saveCalls)
	}

	approved := testApprovedEvidence(t, meta.ID(2003), profileRecord, now.Add(-2*time.Hour))
	other := testDraftProfile(t, meta.ID(1003), now.Add(-time.Hour))
	evaluations.records[approved.ID()] = approved
	profiles.records[profileKey(other.ProfileID(), other.Version())] = other
	_, err = service.Publish(context.Background(), PublishCommand{
		ProfileID: other.ProfileID(), ProfileVersion: other.Version(), EvaluationRunID: approved.ID(), Actor: "owner", Reason: "wrong release",
	})
	if !errors.Is(err, ErrReleaseMismatch) {
		t.Fatalf("mismatched evidence error = %v", err)
	}
}

func TestSameResolutionSlotAllowsSpecificityPrecedenceButRejectsPeers(t *testing.T) {
	base := domainprofile.Selector{Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale, DecisionKind: modelcatalog.DecisionKindScoreRange}
	codeA, codeB, version1, version2 := "model-a", "model-b", "v1", "v2"
	if !sameResolutionSlot(base, base) {
		t.Fatal("two generic selectors must occupy the same slot")
	}
	byCodeA := base
	byCodeA.ModelCode = &codeA
	byCodeA2 := base
	byCodeA2.ModelCode = &codeA
	byCodeB := base
	byCodeB.ModelCode = &codeB
	if !sameResolutionSlot(byCodeA, byCodeA2) || sameResolutionSlot(byCodeA, byCodeB) || sameResolutionSlot(base, byCodeA) {
		t.Fatal("code specificity slot comparison is invalid")
	}
	exactV1 := byCodeA
	exactV1.ModelVersion = &version1
	exactV1Copy := byCodeA
	exactV1Copy.ModelVersion = &version1
	exactV2 := byCodeA
	exactV2.ModelVersion = &version2
	if !sameResolutionSlot(exactV1, exactV1Copy) || sameResolutionSlot(exactV1, exactV2) || sameResolutionSlot(byCodeA, exactV1) {
		t.Fatal("exact specificity slot comparison is invalid")
	}
}

func testDraftProfile(t *testing.T, id meta.ID, at time.Time) *domainprofile.AIExplanationProfile {
	t.Helper()
	suite, err := appevaluation.LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	definition := suite.ProfileFixture.Definition
	definition.ProfileID = "profile-" + id.String()
	profileRecord, err := domainprofile.NewDraft(id, definition, at)
	if err != nil {
		t.Fatal(err)
	}
	return profileRecord
}

func testRelease(profileRecord *domainprofile.AIExplanationProfile) domainevaluation.ReleaseIdentity {
	definition := profileRecord.Definition()
	caseIDs := []string{"generation-1", "generation-2", "generation-3", "generation-4", "generation-5", "generation-6", "generation-7"}
	return domainevaluation.ReleaseIdentity{
		Suite:        domainevaluation.SuiteRef{ID: appevaluation.SuiteIDV1, Version: appevaluation.SuiteVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("suite")), GitBlobSHA: "suite-blob"},
		Prompt:       aiexplanation.PromptRef{TemplateID: definition.GenerationPolicy.PromptTemplateID, Version: definition.GenerationPolicy.PromptVersion, Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "prompt-blob"},
		Profile:      aiexplanation.ProfileRef{ID: profileRecord.ProfileID(), Version: profileRecord.Version(), Fingerprint: profileRecord.Fingerprint()},
		InputSchema:  domainevaluation.SchemaRef{Version: definition.GenerationPolicy.InputSchemaVersion, Fingerprint: aiexplanation.NewFingerprint([]byte("input-schema"))},
		OutputSchema: domainevaluation.SchemaRef{Version: definition.GenerationPolicy.OutputSchemaVersion, Fingerprint: aiexplanation.NewFingerprint([]byte("output-schema"))},
		Provider:     aiexplanation.ProviderExecutionSpec{Route: definition.GenerationPolicy.ProviderRoute, RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("provider-route"))},
		Decoding:     domainevaluation.DecodingParameters{MaxOutputTokens: 3000}, GenerationCaseIDs: caseIDs,
		SemanticEvaluator: governanceSemanticEvaluator(),
		PreflightCaseID:   "preflight", PreflightRejectionReason: "insufficient_eligible_dimensions", RepetitionsPerCase: 5,
	}
}

func testApprovedEvidence(t *testing.T, id meta.ID, profileRecord *domainprofile.AIExplanationProfile, at time.Time) *domainevaluation.PromptEvaluationRun {
	t.Helper()
	release := testRelease(profileRecord)
	run, err := domainevaluation.New(id, release, at)
	if err != nil {
		t.Fatal(err)
	}
	for caseIndex, caseID := range release.GenerationCaseIDs {
		for attempt := 1; attempt <= release.RepetitionsPerCase; attempt++ {
			normalized := []byte(`{"summary":"synthetic"}`)
			receipt := aiexplanation.ProviderReceipt{InvocationID: caseID + "-attempt", RequestID: "request", Provider: "provider-a", Model: "model-a", Latency: time.Second}
			err := run.AddAttempt(domainevaluation.AttemptRecord{
				CaseID: caseID, Attempt: attempt, Stage: domainevaluation.AttemptStageGeneration,
				StartedAt: at.Add(time.Duration(caseIndex*10+attempt) * time.Minute), FinishedAt: at.Add(time.Duration(caseIndex*10+attempt)*time.Minute + time.Second),
				ProviderCallCount: 1, ProviderReceipt: &receipt, RawOutput: normalized, NormalizedOutput: normalized, OutputFingerprint: aiexplanation.NewFingerprint(normalized),
				Assertions: []domainevaluation.AssertionReceipt{
					{Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "contract-v1", Status: domainevaluation.AssertionPassed},
					{Type: "case_goal", Scope: domainevaluation.AssertionScopeCase, Ordinal: 1, Evaluator: "semantic-v1", Status: domainevaluation.AssertionPassed},
				},
				Semantic: &domainevaluation.SemanticReceipt{
					EvaluatorVersion: "semantic-rubric-v1",
					ProviderReceipt:  aiexplanation.ProviderReceipt{InvocationID: "semantic-" + caseID, RequestID: "semantic-request", Provider: "judge-provider", Model: "judge-model", Latency: time.Second},
					Rationale:        "reviewed", Scores: domainevaluation.SemanticScores{Faithfulness: 5, CrossDimensionQuality: 5, SuggestionActionability: 5, AudienceClarity: 5, Concision: 5},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := run.AddAttempt(domainevaluation.AttemptRecord{
		CaseID: release.PreflightCaseID, Attempt: 1, Stage: domainevaluation.AttemptStagePreflight,
		StartedAt: at.Add(80 * time.Minute), FinishedAt: at.Add(80*time.Minute + time.Second), ProviderCallCount: 0, RejectionReason: release.PreflightRejectionReason,
		Assertions: []domainevaluation.AssertionReceipt{
			{Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
			{Type: "rejection_reason", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
		},
	}); err != nil {
		t.Fatal(err)
	}
	closedAt := at.Add(90 * time.Minute)
	if err := run.CloseCollection(closedAt); err != nil {
		t.Fatal(err)
	}
	for _, caseID := range release.GenerationCaseIDs {
		for attempt := 1; attempt <= release.RepetitionsPerCase; attempt++ {
			for _, role := range []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct} {
				if err := run.AddHumanReview(domainevaluation.HumanReview{CaseID: caseID, Attempt: attempt, Role: role, Reviewer: string(role), Decision: domainevaluation.ReviewDecisionApprove, ReviewedAt: closedAt.Add(time.Minute), Reason: "approved"}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := run.Finalize("release-owner", "all gates passed", closedAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return run
}

func governanceSemanticEvaluator() domainevaluation.SemanticEvaluatorSpec {
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

type profileRepositoryStub struct {
	records   map[string]*domainprofile.AIExplanationProfile
	published []*domainprofile.AIExplanationProfile
	saveCalls int
}

func (r *profileRepositoryStub) Save(_ context.Context, value *domainprofile.AIExplanationProfile) error {
	r.records[profileKey(value.ProfileID(), value.Version())] = value
	r.saveCalls++
	return nil
}
func (r *profileRepositoryStub) FindByKey(_ context.Context, id, version string) (*domainprofile.AIExplanationProfile, error) {
	value, ok := r.records[profileKey(id, version)]
	if !ok {
		return nil, domainprofile.ErrNotFound
	}
	return value, nil
}
func (r *profileRepositoryStub) ListPublishedByBaseSelector(context.Context, policy.Audience, modelcatalog.Kind, modelcatalog.DecisionKind) ([]*domainprofile.AIExplanationProfile, error) {
	return append([]*domainprofile.AIExplanationProfile(nil), r.published...), nil
}

type evaluationRepositoryStub struct {
	records map[meta.ID]*domainevaluation.PromptEvaluationRun
}

func (r *evaluationRepositoryStub) Create(_ context.Context, value *domainevaluation.PromptEvaluationRun) error {
	r.records[value.ID()] = value
	return nil
}
func (r *evaluationRepositoryStub) Save(_ context.Context, value *domainevaluation.PromptEvaluationRun, _ int64) error {
	r.records[value.ID()] = value
	return nil
}
func (r *evaluationRepositoryStub) FindByID(_ context.Context, id meta.ID) (*domainevaluation.PromptEvaluationRun, error) {
	value, ok := r.records[id]
	if !ok {
		return nil, domainevaluation.ErrNotFound
	}
	return value, nil
}

func profileKey(id, version string) string { return id + "\x00" + version }
