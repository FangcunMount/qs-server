package participant

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	appsource "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestValidateRequestRejectsMoreThanThreeFocusAreas(t *testing.T) {
	err := validateRequest(Actor{SubjectID: "user-1", TesteeID: 9}, RequestInput{
		AssessmentID: meta.FromUint64(7), Locale: "zh-CN", FocusAreas: []string{"a", "b", "c", "d"},
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("validation error = %v", err)
	}
}

func TestCapabilityAuthorizesBeforeReadingSource(t *testing.T) {
	denied := errors.New("denied")
	fixture := newServiceFixture(t)
	fixture.access.err = denied
	result, err := fixture.service.Capability(context.Background(), fixture.actor, fixture.request)
	if result != nil || !errors.Is(err, denied) {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	if fixture.sources.calls != 0 || fixture.store.commitCalls != 0 {
		t.Fatalf("source/commit calls = %d/%d", fixture.sources.calls, fixture.store.commitCalls)
	}
}

func TestCapabilityReportsNotReadyAndProfileUnresolvedWithoutCreatingGeneration(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.sources.err = appsource.ErrNotReady
	result, err := fixture.service.Capability(context.Background(), fixture.actor, fixture.request)
	if err != nil || result.Status != StatusNotReady || result.ReasonCode != "standard_report_not_ready" {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	fixture = newServiceFixture(t)
	fixture.profiles.items = nil
	result, err = fixture.service.Capability(context.Background(), fixture.actor, fixture.request)
	if err != nil || result.Status != StatusNotApplicable || result.ReasonCode != "profile_unresolved" {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	if fixture.store.commitCalls != 0 {
		t.Fatal("capability check created a generation")
	}
}

func TestRequestCreatesOnePendingSemanticGeneration(t *testing.T) {
	fixture := newServiceFixture(t)
	result, err := fixture.service.Request(context.Background(), fixture.actor, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusPending || result.GenerationID.IsZero() || result.SourceState != SourceStateCurrent {
		t.Fatalf("result = %#v", result)
	}
	created := fixture.store.lastCommitted
	if created == nil || created.ID() != result.GenerationID {
		t.Fatalf("created generation = %#v", created)
	}
	if created.RequestedBy().Kind != "participant" || created.RequestedBy().ID != fixture.actor.SubjectID {
		t.Fatalf("requested by = %#v", created.RequestedBy())
	}
	if created.Key().SourceReportID != fixture.sources.current.Report.ID() || created.Key().InputFingerprint != created.Input().Fingerprint() {
		t.Fatalf("generation key = %#v", created.Key())
	}
	if fixture.prompts.calls != 1 || fixture.routes.calls != 1 || fixture.store.commitCalls != 1 {
		t.Fatalf("prompt/route/commit calls = %d/%d/%d", fixture.prompts.calls, fixture.routes.calls, fixture.store.commitCalls)
	}

	again, err := fixture.service.Request(context.Background(), fixture.actor, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if again.GenerationID != result.GenerationID || again.Status != StatusPending {
		t.Fatalf("duplicate result = %#v, want generation %s", again, result.GenerationID)
	}
	if fixture.store.commitCalls != 1 || len(fixture.store.byID) != 1 {
		t.Fatalf("duplicate commit calls/generations = %d/%d", fixture.store.commitCalls, len(fixture.store.byID))
	}
}

func TestRequestReturnsCapacityErrorOnlyWhenNoSemanticGenerationExists(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.store.commitErr = domaingeneration.ErrUserDailyBudgetExceeded
	result, err := fixture.service.Request(context.Background(), fixture.actor, fixture.request)
	if result != nil || !errors.Is(err, domaingeneration.ErrUserDailyBudgetExceeded) {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	if fixture.store.commitCalls != 1 || len(fixture.store.byID) != 0 {
		t.Fatalf("capacity rejection commit calls/generations = %d/%d", fixture.store.commitCalls, len(fixture.store.byID))
	}
}

func TestRequestReusesConcurrentWinnerWhenAdmissionReportsCapacity(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.store.commitErr = domaingeneration.ErrOrgDailyBudgetExceeded
	fixture.store.persistBeforeError = true
	result, err := fixture.service.Request(context.Background(), fixture.actor, fixture.request)
	if err != nil || result == nil || result.Status != StatusPending || result.GenerationID.IsZero() {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	if fixture.store.commitCalls != 1 || len(fixture.store.byID) != 1 {
		t.Fatalf("concurrent reuse commit calls/generations = %d/%d", fixture.store.commitCalls, len(fixture.store.byID))
	}
}

func TestGetScopesGenerationToAuthorizedAssessmentAndReportsStaleSource(t *testing.T) {
	fixture := newServiceFixture(t)
	created, err := fixture.service.Request(context.Background(), fixture.actor, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.catalog.metadata.SourceID = fixture.sources.current.Report.ID().Uint64() + 1
	result, err := fixture.service.Get(context.Background(), fixture.actor, GetInput{AssessmentID: fixture.request.AssessmentID, GenerationID: created.GenerationID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusPending || result.SourceState != SourceStateStale {
		t.Fatalf("result = %#v", result)
	}

	foreignActor := Actor{SubjectID: "other-user", TesteeID: fixture.actor.TesteeID + 1}
	_, err = fixture.service.Get(context.Background(), foreignActor, GetInput{AssessmentID: fixture.request.AssessmentID, GenerationID: created.GenerationID})
	if !errors.Is(err, ErrAccessMismatch) {
		t.Fatalf("foreign error = %v", err)
	}
}

func TestGetReturnsValidatedGeneratedArtifact(t *testing.T) {
	fixture := newServiceFixture(t)
	created, err := fixture.service.Request(context.Background(), fixture.actor, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	generationRecord := fixture.store.byID[created.GenerationID]
	runID := meta.FromUint64(800)
	artifactID := meta.FromUint64(900)
	if err := generationRecord.Begin(runID, fixture.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Succeed(runID, artifactID, fixture.now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	fixture.store.byID[generationRecord.ID()] = generationRecord
	content := output.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1,
		Summary:       "本次结果显示睡眠与压力维度可能相互影响。",
		IntegratedInsights: []output.IntegratedInsight{{
			Kind: output.InsightKindReinforcingPattern, Title: "组合关注", Content: "两个维度可结合观察。", WhyItMatters: "有助于理解本次结果。",
			EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:sleep"}, {Kind: output.EvidenceKindDimension, Ref: "dimension:stress"}},
		}},
		Suggestions: []output.Suggestion{{
			Origin: output.SuggestionOriginGeneratedLowRisk, Category: "routine", Title: "简短记录", Goal: "观察日常变化", Actions: []string{"每天记录一次"},
			Rationale: "便于结合本次结果观察。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:sleep"}},
		}},
		Limitations: []string{"仅基于本次测评，不构成诊断或确定性判断。"},
	}
	source := fixture.sources.current.Report
	receipt := aiexplanation.ProviderReceipt{InvocationID: "inv-1", Provider: fixture.routes.route.ExecutionSpec.ResolvedProvider, Model: fixture.routes.route.ExecutionSpec.ResolvedModel, InputTokens: 100, OutputTokens: 200, Latency: time.Second}
	artifactRecord, err := domainartifact.New(domainartifact.NewInput{
		ID: artifactID, GenerationID: generationRecord.ID(), RunID: runID,
		Source: domainartifact.SourceRef{
			ReportID: source.ID(), OutcomeID: source.OutcomeID(), Association: generationRecord.Association(), ReportType: source.ReportType().String(),
			TemplateVersion: source.TemplateVersion().String(), ContentSchemaVersion: source.ContentSchemaVersion(), BuilderIdentity: source.BuilderIdentity(), ReportGeneratedAt: source.GeneratedAt(),
		},
		Audience: generationRecord.Key().Audience, Profile: generationRecord.Key().Profile, Prompt: generationRecord.Prompt(), ExecutionSpec: generationRecord.ExecutionSpec(),
		InputSchema: aiexplanation.InputSchemaVersionV1, InputFingerprint: generationRecord.Input().Fingerprint(), OutputSchema: aiexplanation.OutputSchemaVersionV1,
		SafetyPolicy: fixture.profiles.items[0].Definition().SafetyPolicy.PolicyVersion, ProviderReceipt: receipt,
		Validation: domainartifact.ValidationReceipt{SchemaValidatorVersion: "v1", ReferenceValidatorVersion: "v1", ProfileValidatorVersion: "v1", SafetyValidatorVersion: "v1", ValidatedAt: fixture.now.Add(3 * time.Second)},
		Content:    content, GeneratedAt: fixture.now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.artifacts.byID[artifactID] = artifactRecord
	result, err := fixture.service.Get(context.Background(), fixture.actor, GetInput{AssessmentID: fixture.request.AssessmentID, GenerationID: generationRecord.ID()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusGenerated || result.Content == nil || result.Content.Summary != content.Summary || result.ArtifactID != artifactID {
		t.Fatalf("result = %#v", result)
	}
}

type serviceFixture struct {
	service   Service
	access    *accessStub
	sources   *sourceStub
	profiles  *profileRepositoryStub
	prompts   *promptResolverStub
	routes    *routeResolverStub
	store     *generationStore
	runs      *runRepositoryStub
	artifacts *artifactRepositoryStub
	catalog   *catalogStub
	actor     Actor
	request   RequestInput
	now       time.Time
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	current := participantCurrentSource(t, now)
	profileRecord := participantProfile(t, current, now)
	fingerprint := aiexplanation.NewFingerprint([]byte("route"))
	fixture := &serviceFixture{
		access: &accessStub{}, sources: &sourceStub{current: current}, profiles: &profileRepositoryStub{items: []*domainprofile.AIExplanationProfile{profileRecord}},
		prompts: &promptResolverStub{pkg: appport.PromptPackage{
			Ref:                 aiexplanation.PromptRef{TemplateID: "cross-dimension-participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "abc123"},
			SystemMessage:       "system",
			TaskTemplate:        "task {{locale}}",
			DataPreamble:        "data",
			AllowedPlaceholders: []string{"{{locale}}"},
		}},
		routes: &routeResolverStub{route: appport.ProviderRoute{
			ExecutionSpec:   aiexplanation.ProviderExecutionSpec{Route: "participant_default", RouteRevision: "v1", ResolvedProvider: "test-provider", ResolvedModel: "test-model", Fingerprint: fingerprint},
			Capabilities:    appport.ProviderCapabilities{StructuredOutput: true},
			Timeout:         30 * time.Second,
			MaxOutputTokens: 2048,
		}},
		store: &generationStore{byID: map[meta.ID]*domaingeneration.AIExplanationGeneration{}, byKey: map[domaingeneration.Key]*domaingeneration.AIExplanationGeneration{}},
		runs:  &runRepositoryStub{}, artifacts: &artifactRepositoryStub{byID: map[meta.ID]*domainartifact.AIExplanationArtifact{}},
		catalog: &catalogStub{metadata: interpretationreadmodel.CurrentReportMetadata{AssessmentID: current.Report.Association().AssessmentID.Uint64(), Status: interpretationreadmodel.CurrentReportMetadataFound, SourceKind: "artifact", SourceID: current.Report.ID().Uint64()}},
		actor:   Actor{SubjectID: "user-1", TesteeID: current.Report.Association().TesteeID},
		request: RequestInput{AssessmentID: current.Report.Association().AssessmentID, Locale: "zh-CN"}, now: now,
	}
	serviceValue, err := NewService(fixture.access, fixture.sources, fixture.profiles, fixture.prompts, fixture.routes, fixture.store, fixture.store, fixture.runs, fixture.artifacts, fixture.catalog)
	if err != nil {
		t.Fatal(err)
	}
	concrete := serviceValue.(*service)
	concrete.now = func() time.Time { return now }
	concrete.newID = func() meta.ID { return meta.FromUint64(700) }
	fixture.service = serviceValue
	return fixture
}

type accessStub struct{ err error }

func (s *accessStub) AuthorizeOwnAssessment(context.Context, uint64, uint64) error { return s.err }

type sourceStub struct {
	current *appsource.Current
	err     error
	calls   int
}

func (s *sourceStub) ResolveCurrent(context.Context, meta.ID) (*appsource.Current, error) {
	s.calls++
	return s.current, s.err
}

type profileRepositoryStub struct {
	items []*domainprofile.AIExplanationProfile
}

func (*profileRepositoryStub) Save(context.Context, *domainprofile.AIExplanationProfile) error {
	return nil
}
func (*profileRepositoryStub) FindByKey(context.Context, string, string) (*domainprofile.AIExplanationProfile, error) {
	return nil, domainprofile.ErrNotFound
}
func (s *profileRepositoryStub) ListPublishedByBaseSelector(context.Context, policy.Audience, modelcatalog.Kind, modelcatalog.DecisionKind) ([]*domainprofile.AIExplanationProfile, error) {
	return s.items, nil
}

type promptResolverStub struct {
	pkg   appport.PromptPackage
	err   error
	calls int
}

func (s *promptResolverStub) ResolvePromptPackage(context.Context, string, string) (appport.PromptPackage, error) {
	s.calls++
	return s.pkg, s.err
}

type routeResolverStub struct {
	route appport.ProviderRoute
	err   error
	calls int
}

func (s *routeResolverStub) ResolveProviderRoute(context.Context, string) (appport.ProviderRoute, error) {
	s.calls++
	return s.route, s.err
}

type generationStore struct {
	byID               map[meta.ID]*domaingeneration.AIExplanationGeneration
	byKey              map[domaingeneration.Key]*domaingeneration.AIExplanationGeneration
	lastCommitted      *domaingeneration.AIExplanationGeneration
	commitCalls        int
	commitErr          error
	persistBeforeError bool
}

func (s *generationStore) CommitRequested(_ context.Context, value *domaingeneration.AIExplanationGeneration) error {
	s.commitCalls++
	if s.commitErr != nil {
		if s.persistBeforeError {
			s.byID[value.ID()] = value
			s.byKey[value.Key()] = value
		}
		return s.commitErr
	}
	if _, exists := s.byKey[value.Key()]; exists {
		return domaingeneration.ErrAlreadyExists
	}
	s.byID[value.ID()] = value
	s.byKey[value.Key()] = value
	s.lastCommitted = value
	return nil
}
func (s *generationStore) Create(ctx context.Context, value *domaingeneration.AIExplanationGeneration) error {
	return s.CommitRequested(ctx, value)
}
func (s *generationStore) FindByID(_ context.Context, id meta.ID) (*domaingeneration.AIExplanationGeneration, error) {
	if value := s.byID[id]; value != nil {
		return value, nil
	}
	return nil, domaingeneration.ErrNotFound
}
func (s *generationStore) FindByKey(_ context.Context, key domaingeneration.Key) (*domaingeneration.AIExplanationGeneration, error) {
	if value := s.byKey[key]; value != nil {
		return value, nil
	}
	return nil, domaingeneration.ErrNotFound
}
func (s *generationStore) Save(_ context.Context, value *domaingeneration.AIExplanationGeneration, _ uint64) error {
	s.byID[value.ID()] = value
	s.byKey[value.Key()] = value
	return nil
}

type runRepositoryStub struct{ latest *domainrun.AIExplanationRun }

func (*runRepositoryStub) Create(context.Context, *domainrun.AIExplanationRun) error { return nil }
func (*runRepositoryStub) FindByID(context.Context, meta.ID) (*domainrun.AIExplanationRun, error) {
	return nil, domainrun.ErrNotFound
}
func (s *runRepositoryStub) FindLatestByGenerationID(context.Context, meta.ID) (*domainrun.AIExplanationRun, error) {
	if s.latest != nil {
		return s.latest, nil
	}
	return nil, domainrun.ErrNotFound
}
func (*runRepositoryStub) Save(context.Context, *domainrun.AIExplanationRun) error { return nil }

type artifactRepositoryStub struct {
	byID map[meta.ID]*domainartifact.AIExplanationArtifact
}

func (*artifactRepositoryStub) Insert(context.Context, *domainartifact.AIExplanationArtifact) error {
	return nil
}
func (s *artifactRepositoryStub) FindByID(_ context.Context, id meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	if value := s.byID[id]; value != nil {
		return value, nil
	}
	return nil, domainartifact.ErrNotFound
}
func (*artifactRepositoryStub) FindByGenerationID(context.Context, meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	return nil, domainartifact.ErrNotFound
}
func (*artifactRepositoryStub) FindBySourceReportAndAudience(context.Context, meta.ID, policy.Audience) (*domainartifact.AIExplanationArtifact, error) {
	return nil, domainartifact.ErrNotFound
}

type catalogStub struct {
	metadata interpretationreadmodel.CurrentReportMetadata
	err      error
}

func (s *catalogStub) GetCurrentReportMetadataByAssessmentIDs(context.Context, []uint64) (map[uint64]interpretationreadmodel.CurrentReportMetadata, error) {
	if s.err != nil {
		return nil, s.err
	}
	return map[uint64]interpretationreadmodel.CurrentReportMetadata{s.metadata.AssessmentID: s.metadata}, nil
}

func participantCurrentSource(t *testing.T, now time.Time) *appsource.Current {
	t.Helper()
	max := 10.0
	model := domainreport.ModelIdentity{Kind: string(modelcatalog.KindScale), Algorithm: string(modelcatalog.AlgorithmScaleDefault), Code: "scale-a", Version: "v1", Title: "Scale A"}
	sleep := domainreport.NewDimensionInterpret(domainreport.NewFactorCode("sleep"), "睡眠", 6, &max, domainreport.RiskLevelLow, "睡眠表现需要关注", "记录睡眠节律").WithHierarchy("factor", "", 1, 1)
	stress := domainreport.NewDimensionInterpret(domainreport.NewFactorCode("stress"), "压力", 7, &max, domainreport.RiskLevelMedium, "压力水平偏高", "安排短暂休息").WithHierarchy("factor", "", 1, 2)
	presentation := domainreport.NewFrozenPresentationProfile([]string{"sleep", "stress"})
	reportRecord, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID: meta.FromUint64(101), GenerationID: meta.FromUint64(201), OutcomeID: meta.FromUint64(301), InterpretationRunID: meta.FromUint64(401),
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 9}, ReportType: policy.ReportTypeStandard,
		TemplateVersion: policy.TemplateVersionCurrent, BuilderIdentity: domainreport.BuilderIdentityFactorScoring, ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		Content: domainreport.Content{
			Model: model, PrimaryScore: &domainreport.ScoreValue{Kind: domainreport.ScoreKindRawTotal, Value: 13, Max: &max},
			Level: &domainreport.ResultLevel{Code: "medium", Label: "中等", Severity: "medium"}, Conclusion: "本次结果可结合压力与睡眠观察。",
			Dimensions: []domainreport.DimensionInterpret{sleep, stress}, PresentationProfile: &presentation,
		},
		GeneratedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	association := reportRecord.Association()
	outcomeRecord := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: reportRecord.OutcomeID(), OrgID: association.OrgID, AssessmentID: association.AssessmentID, TesteeID: association.TesteeID,
		Model:   evaluationfact.ModelIdentity{Kind: modelcatalog.KindScale, Algorithm: modelcatalog.AlgorithmScaleDefault, Code: model.Code, Version: model.Version, Title: model.Title},
		Runtime: evaluationfact.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindScoreRange}, EvaluatedAt: now.Add(-time.Second),
	})
	return &appsource.Current{Report: reportRecord, Outcome: outcomeRecord}
}

func participantProfile(t *testing.T, current *appsource.Current, now time.Time) *domainprofile.AIExplanationProfile {
	t.Helper()
	definition := domainprofile.Definition{
		SchemaVersion: aiexplanation.ProfileSchemaVersionV1, ProfileID: "participant-scale", Version: "v1",
		Selector:      domainprofile.Selector{Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale, DecisionKind: modelcatalog.DecisionKindScoreRange},
		Eligibility:   domainprofile.EligibilityPolicy{MinEligibleDimensions: 2, MaxInputDimensions: 50, OnDimensionOverflow: "reject"},
		InputPolicy:   domainprofile.InputPolicy{ContextScope: "current_assessment_only", IncludeNormContext: true},
		InsightPolicy: domainprofile.InsightPolicy{AllowedKinds: []output.InsightKind{output.InsightKindReinforcingPattern}, MinItems: 1, MaxItems: 3, MinDimensionRefsPerItem: 2, MaxDimensionRefsPerItem: 4},
		SuggestionPolicy: domainprofile.SuggestionPolicy{
			AllowedOrigins: []output.SuggestionOrigin{output.SuggestionOriginGeneratedLowRisk}, AllowedCategories: []string{"routine"}, MinItems: 1, MaxItems: 3, MaxActionsPerItem: 3,
			RequireEvidenceRefs: true, RequireStandardRefsForStandardDerived: true,
		},
		SafetyPolicy:     domainprofile.SafetyPolicy{PolicyVersion: "v1", DisclaimerVersion: "v1", ForbiddenClaims: []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"}},
		GenerationPolicy: domainprofile.GenerationPolicy{PromptTemplateID: "cross-dimension-participant-scale", PromptVersion: "v1", ProviderRoute: "participant_default", InputSchemaVersion: aiexplanation.InputSchemaVersionV1, OutputSchemaVersion: aiexplanation.OutputSchemaVersionV1, MaxOutputCharacters: 4000},
	}
	profileRecord, err := domainprofile.NewDraft(meta.FromUint64(501), definition, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := profileRecord.Publish(meta.ID(101), "tester", "approved evaluation", now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return profileRecord
}
