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
	evidence := testApprovedEvidenceV2(profileRecord, meta.ID(2001), now.Add(-2*time.Hour))
	profiles := &profileRepositoryStub{records: map[string]*domainprofile.AIExplanationProfile{profileKey(profileRecord.ProfileID(), profileRecord.Version()): profileRecord}}
	evaluations := &evaluationRepositoryStub{evidenceV2Records: map[meta.ID]*domainevaluation.PromptEvaluationEvidenceV2{evidence.RunID: evidence}}
	service, err := NewService(profiles, evaluations, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	published, err := service.Publish(context.Background(), PublishCommand{
		ProfileID: profileRecord.ProfileID(), ProfileVersion: profileRecord.Version(), EvaluationRunID: evidence.RunID,
		Actor: "release-owner", Reason: "rubric and dual review passed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if published.Status() != domainprofile.StatusPublished || published.PublishedEvidenceRunID() != evidence.RunID || published.PublishedBy() != "release-owner" || published.PublishedReason() != "rubric and dual review passed" {
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
	if disabled.Status() != domainprofile.StatusDisabled || disabled.DisabledReason() != "route retired" || disabled.PublishedEvidenceRunID() != evidence.RunID {
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
	service, err := NewService(profiles, &evaluationRepositoryStub{evidenceV2Records: map[meta.ID]*domainevaluation.PromptEvaluationEvidenceV2{}}, func() time.Time { return now }, func() meta.ID { return meta.ID(3001) })
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

func TestProfileCatalogFindsVersionAndUsesStableStatusPage(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	profileRecord := testDraftProfile(t, meta.ID(1010), now)
	profiles := &profileRepositoryStub{
		records: map[string]*domainprofile.AIExplanationProfile{profileKey(profileRecord.ProfileID(), profileRecord.Version()): profileRecord},
		catalog: []*domainprofile.AIExplanationProfile{profileRecord}, nextCursor: "next-profile-page",
	}
	service, err := NewService(profiles, &evaluationRepositoryStub{evidenceV2Records: map[meta.ID]*domainevaluation.PromptEvaluationEvidenceV2{}}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	found, err := service.Find(context.Background(), profileRecord.ProfileID(), profileRecord.Version())
	if err != nil || found != profileRecord {
		t.Fatalf("find Profile = %#v, err = %v", found, err)
	}
	status := domainprofile.StatusDraft
	page, err := service.List(context.Background(), ProfileListQuery{Status: &status, Cursor: "current", Limit: 7})
	if err != nil {
		t.Fatal(err)
	}
	if profiles.catalogStatus == nil || *profiles.catalogStatus != status || profiles.catalogCursor != "current" || profiles.catalogLimit != 7 ||
		len(page.Items) != 1 || page.Items[0] != profileRecord || page.NextCursor != "next-profile-page" {
		t.Fatalf("Profile catalog query/page = status:%v cursor:%q limit:%d page:%#v", profiles.catalogStatus, profiles.catalogCursor, profiles.catalogLimit, page)
	}

	profiles.catalog = []*domainprofile.AIExplanationProfile{testPublishedProfile(t, meta.ID(1011), now)}
	if _, err := service.List(context.Background(), ProfileListQuery{Status: &status, Limit: 7}); err == nil {
		t.Fatal("Profile catalog must reject a result that does not match the status filter")
	}
}

func TestPublishRejectsIncompleteOrMismatchedEvidence(t *testing.T) {
	now := time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC)
	profileRecord := testDraftProfile(t, meta.ID(1002), now.Add(-time.Hour))
	incomplete := testEvaluationEvidenceV2(profileRecord, meta.ID(2002), domainevaluation.EvidenceStatusAwaitingReview, now.Add(-2*time.Hour))
	profiles := &profileRepositoryStub{records: map[string]*domainprofile.AIExplanationProfile{profileKey(profileRecord.ProfileID(), profileRecord.Version()): profileRecord}}
	evaluations := &evaluationRepositoryStub{evidenceV2Records: map[meta.ID]*domainevaluation.PromptEvaluationEvidenceV2{incomplete.RunID: incomplete}}
	service, _ := NewService(profiles, evaluations, func() time.Time { return now })
	_, err := service.Publish(context.Background(), PublishCommand{
		ProfileID: profileRecord.ProfileID(), ProfileVersion: profileRecord.Version(), EvaluationRunID: incomplete.RunID, Actor: "owner", Reason: "try",
	})
	if !errors.Is(err, ErrPublishEvidenceRequired) || profiles.saveCalls != 0 {
		t.Fatalf("incomplete evidence error/save calls = %v/%d", err, profiles.saveCalls)
	}

	legacyMasquerade := testApprovedEvidenceV2(profileRecord, meta.ID(2004), now.Add(-2*time.Hour))
	legacyMasquerade.SchemaVersion = "prompt-evaluation-evidence/v1"
	evaluations.evidenceV2Records[legacyMasquerade.RunID] = legacyMasquerade
	_, err = service.Publish(context.Background(), PublishCommand{
		ProfileID: profileRecord.ProfileID(), ProfileVersion: profileRecord.Version(), EvaluationRunID: legacyMasquerade.RunID, Actor: "owner", Reason: "legacy evidence",
	})
	if !errors.Is(err, ErrPublishEvidenceRequired) || profiles.saveCalls != 0 {
		t.Fatalf("legacy evidence error/save calls = %v/%d", err, profiles.saveCalls)
	}

	approved := testApprovedEvidenceV2(profileRecord, meta.ID(2003), now.Add(-2*time.Hour))
	other := testDraftProfile(t, meta.ID(1003), now.Add(-time.Hour))
	evaluations.evidenceV2Records[approved.RunID] = approved
	profiles.records[profileKey(other.ProfileID(), other.Version())] = other
	_, err = service.Publish(context.Background(), PublishCommand{
		ProfileID: other.ProfileID(), ProfileVersion: other.Version(), EvaluationRunID: approved.RunID, Actor: "owner", Reason: "wrong release",
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

func testPublishedProfile(t *testing.T, id meta.ID, at time.Time) *domainprofile.AIExplanationProfile {
	t.Helper()
	value := testDraftProfile(t, id, at)
	if err := value.Publish(meta.ID(7000)+id, "release-owner", "approved evidence", at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	return value
}

func testApprovedEvidenceV2(profileRecord *domainprofile.AIExplanationProfile, id meta.ID, at time.Time) *domainevaluation.PromptEvaluationEvidenceV2 {
	return testEvaluationEvidenceV2(profileRecord, id, domainevaluation.EvidenceStatusApproved, at)
}

func testEvaluationEvidenceV2(profileRecord *domainprofile.AIExplanationProfile, id meta.ID, status domainevaluation.EvidenceStatus, at time.Time) *domainevaluation.PromptEvaluationEvidenceV2 {
	definition := profileRecord.Definition()
	fingerprint := func(seed string) aiexplanation.Fingerprint { return aiexplanation.NewFingerprint([]byte(seed)) }
	closedAt, finalizedAt := at.Add(time.Hour), at.Add(2*time.Hour)
	value := &domainevaluation.PromptEvaluationEvidenceV2{
		SchemaVersion:   domainevaluation.PromptEvaluationEvidenceSchemaVersionV2,
		RunID:           id,
		Status:          status,
		ExecutionPolicy: domainevaluation.CurrentEvaluationExecutionPolicy(),
		GatePolicy:      domainevaluation.CurrentReleaseGatePolicy(),
		Release: domainevaluation.EvidenceReleaseIdentity{
			Fingerprint:     fingerprint("release"),
			Suite:           domainevaluation.FrozenContractRef{ID: appevaluation.SuiteIDV1, Version: appevaluation.SuiteVersionV1, Fingerprint: fingerprint("suite")},
			Prompt:          domainevaluation.FrozenContractRef{ID: definition.GenerationPolicy.PromptTemplateID, Version: definition.GenerationPolicy.PromptVersion, Fingerprint: fingerprint("prompt")},
			Profile:         domainevaluation.FrozenContractRef{ID: profileRecord.ProfileID(), Version: profileRecord.Version(), Fingerprint: profileRecord.Fingerprint()},
			InputSchema:     domainevaluation.FrozenContractRef{ID: "ai-explanation-input", Version: definition.GenerationPolicy.InputSchemaVersion, Fingerprint: fingerprint("input")},
			OutputSchema:    domainevaluation.FrozenContractRef{ID: "ai-explanation-output", Version: definition.GenerationPolicy.OutputSchemaVersion, Fingerprint: fingerprint("output")},
			GenerationRoute: domainevaluation.FrozenContractRef{ID: definition.GenerationPolicy.ProviderRoute, Version: "v1", Fingerprint: fingerprint("generation-route")},
		},
		Audit: domainevaluation.EvidenceRunAudit{
			OrganizationID: 7, RequestedBy: "owner", RequestReason: "evaluate", CreatedAt: at, ClosedAt: &closedAt,
		},
	}
	if status == domainevaluation.EvidenceStatusApproved {
		value.Audit.FinalizedAt = &finalizedAt
		value.GateResult = &domainevaluation.EvidenceGateResult{
			EvaluatedAt: finalizedAt, Passed: true,
			GatePasses: map[string]bool{"G1": true, "G2": true, "G3": true, "G4": true, "G5": true},
		}
	}
	return value
}

type profileRepositoryStub struct {
	records       map[string]*domainprofile.AIExplanationProfile
	published     []*domainprofile.AIExplanationProfile
	catalog       []*domainprofile.AIExplanationProfile
	nextCursor    string
	catalogStatus *domainprofile.Status
	catalogCursor string
	catalogLimit  int
	saveCalls     int
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
func (r *profileRepositoryStub) ListProfiles(_ context.Context, status *domainprofile.Status, cursor string, limit int) ([]*domainprofile.AIExplanationProfile, string, error) {
	r.catalogStatus, r.catalogCursor, r.catalogLimit = status, cursor, limit
	return append([]*domainprofile.AIExplanationProfile(nil), r.catalog...), r.nextCursor, nil
}

type evaluationRepositoryStub struct {
	records           map[meta.ID]*domainevaluation.PromptEvaluationRun
	evidenceV2Records map[meta.ID]*domainevaluation.PromptEvaluationEvidenceV2
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

func (r *evaluationRepositoryStub) CreateEvidenceV2(_ context.Context, value *domainevaluation.PromptEvaluationEvidenceV2) error {
	if r.evidenceV2Records == nil {
		r.evidenceV2Records = make(map[meta.ID]*domainevaluation.PromptEvaluationEvidenceV2)
	}
	r.evidenceV2Records[value.RunID] = value
	return nil
}

func (r *evaluationRepositoryStub) SaveEvidenceV2(_ context.Context, value *domainevaluation.PromptEvaluationEvidenceV2, _ int64) error {
	return r.CreateEvidenceV2(context.Background(), value)
}

func (r *evaluationRepositoryStub) FindEvidenceV2ByID(_ context.Context, id meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	value, ok := r.evidenceV2Records[id]
	if !ok {
		return nil, domainevaluation.ErrNotFound
	}
	return value, nil
}

func profileKey(id, version string) string { return id + "\x00" + version }
