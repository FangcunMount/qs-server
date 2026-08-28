package administration

import (
	"context"
	"testing"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appgovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	apprecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestRecordReviewDerivesReviewerFromTrustedActor(t *testing.T) {
	reviews := &reviewWorkflowStub{run: reviewRunFixture()}
	access := &accessStub{}
	service := NewService(reviews, &governanceStub{}, access)

	result, err := service.RecordReview(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(9), ReviewCommand{
		CaseID: "PROMPT-EVAL-001", Attempt: 1, Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Decision: domainevaluation.ReviewDecisionApprove, Reason: "facts and direction match",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || reviews.recorded.Reviewer != "user:42" {
		t.Fatalf("trusted reviewer = %q", reviews.recorded.Reviewer)
	}
	if access.reviewRole != domainevaluation.ReviewRoleAssessmentSemantics {
		t.Fatalf("authorized role = %q", access.reviewRole)
	}
}

func TestRecordReviewRejectsSameReviewerAcrossRolesBeforeMutation(t *testing.T) {
	run := reviewRunFixture()
	run.Attempts[0].Reviews = []domainevaluation.HumanReview{{
		CaseID: "PROMPT-EVAL-001", Attempt: 1, Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Reviewer: "user:42", Decision: domainevaluation.ReviewDecisionApprove, ReviewedAt: time.Now(), Reason: "reviewed",
	}}
	run.Attempts[0].MissingRoles = []domainevaluation.ReviewRole{domainevaluation.ReviewRoleSafetyProduct}
	reviews := &reviewWorkflowStub{run: run}
	service := NewService(reviews, &governanceStub{}, &accessStub{})

	_, err := service.RecordReview(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, meta.ID(9), ReviewCommand{
		CaseID: "PROMPT-EVAL-001", Attempt: 1, Role: domainevaluation.ReviewRoleSafetyProduct,
		Decision: domainevaluation.ReviewDecisionApprove, Reason: "safe",
	})
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrConflict || reviews.recordCalls != 0 {
		t.Fatalf("same-reviewer result = %v, record calls = %d", err, reviews.recordCalls)
	}
}

func TestCreateProfileDraftPassesFingerprintAndTrustedAudit(t *testing.T) {
	definition := profileDefinitionFixture(t)
	fingerprint, err := definition.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	governance := &governanceStub{}
	service := NewService(&reviewWorkflowStub{}, governance, &accessStub{})
	_, err = service.CreateProfileDraft(context.Background(), Actor{OrgID: 7, OperatorUserID: 88}, CreateProfileDraftCommand{
		Definition: definition, ExpectedFingerprint: fingerprint, Reason: "initial reviewed release candidate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if governance.created.Actor != "user:88" || governance.created.ExpectedFingerprint != fingerprint {
		t.Fatalf("draft audit/fingerprint = %#v", governance.created)
	}
}

func TestGovernanceCatalogReadsUseTrustedActorAndAuditAuthorization(t *testing.T) {
	profileDefinition := profileDefinitionFixture(t)
	profileRecord, err := domainprofile.NewDraftForRelease(meta.ID(81), profileDefinition, "user:42", "review candidate", time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	reviews := &reviewWorkflowStub{listPage: &appevaluation.ReviewRunPage{
		Items:      []*appevaluation.ReviewRun{{RunID: meta.ID(9), RequestedOrgID: 7, Status: domainevaluation.StatusAwaitingReview}},
		NextCursor: "next-evaluation-page",
	}}
	governance := &governanceStub{
		profile:  profileRecord,
		listPage: &appgovernance.ProfilePage{Items: []*domainprofile.AIExplanationProfile{profileRecord}, NextCursor: "next-profile-page"},
	}
	access := &accessStub{}
	service := NewService(reviews, governance, access)
	actor := Actor{OrgID: 7, OperatorUserID: 42}
	evaluationStatus := domainevaluation.StatusAwaitingReview
	evaluations, err := service.ListEvaluations(context.Background(), actor, EvaluationListQuery{Status: &evaluationStatus, Cursor: "current-evaluation", Limit: 7})
	if err != nil {
		t.Fatal(err)
	}
	if reviews.listQuery.OrgID != 7 || reviews.listQuery.Status == nil || *reviews.listQuery.Status != evaluationStatus || reviews.listQuery.Cursor != "current-evaluation" || reviews.listQuery.Limit != 7 ||
		evaluations.NextCursor != "next-evaluation-page" {
		t.Fatalf("evaluation catalog query/page = %#v / %#v", reviews.listQuery, evaluations)
	}

	profileStatus := domainprofile.StatusDraft
	profiles, err := service.ListProfiles(context.Background(), actor, ProfileListQuery{Status: &profileStatus, Cursor: "current-profile", Limit: 9})
	if err != nil {
		t.Fatal(err)
	}
	if governance.listQuery.Status == nil || *governance.listQuery.Status != profileStatus || governance.listQuery.Cursor != "current-profile" || governance.listQuery.Limit != 9 ||
		profiles.NextCursor != "next-profile-page" {
		t.Fatalf("Profile catalog query/page = %#v / %#v", governance.listQuery, profiles)
	}
	found, err := service.FindProfile(context.Background(), actor, profileRecord.ProfileID(), profileRecord.Version())
	if err != nil || found != profileRecord || governance.foundProfileID != profileRecord.ProfileID() || governance.foundVersion != profileRecord.Version() {
		t.Fatalf("Profile find = %#v key=%s/%s err=%v", found, governance.foundProfileID, governance.foundVersion, err)
	}
	if access.readCalls != 3 || access.readActor != actor {
		t.Fatalf("catalog audit authorization = calls:%d actor:%#v", access.readCalls, access.readActor)
	}
}

func TestGovernanceCatalogMapsOpaqueCursorFailuresToInvalidArgument(t *testing.T) {
	actor := Actor{OrgID: 7, OperatorUserID: 42}
	reviews := &reviewWorkflowStub{listErr: appevaluation.ErrReviewCatalogCursor}
	governance := &governanceStub{listErr: appgovernance.ErrProfileCatalogCursor}
	service := NewService(reviews, governance, &accessStub{})

	if _, err := service.ListEvaluations(context.Background(), actor, EvaluationListQuery{Cursor: "invalid"}); cberrors.ParseCoder(err) == nil || cberrors.ParseCoder(err).Code() != code.ErrInvalidArgument {
		t.Fatalf("evaluation cursor error = %v", err)
	}
	if _, err := service.ListProfiles(context.Background(), actor, ProfileListQuery{Cursor: "invalid"}); cberrors.ParseCoder(err) == nil || cberrors.ParseCoder(err).Code() != code.ErrInvalidArgument {
		t.Fatalf("Profile cursor error = %v", err)
	}
}

func TestStartEvaluationRequiresExplicitCostConfirmationAndTrustedAudit(t *testing.T) {
	reviews := &reviewWorkflowStub{run: &appevaluation.ReviewRun{RunID: meta.ID(700), Status: domainevaluation.StatusCollecting}}
	starter := &evaluationStarterStub{}
	committer := &evaluationStartCommitterStub{}
	service := NewService(reviews, &governanceStub{}, &accessStub{}, WithEvaluationExecution(
		starter, committer, func() meta.ID { return meta.ID(700) },
	))

	_, err := service.StartEvaluation(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, StartEvaluationCommand{
		Confirm: true, ExpectedProviderInvocations: appevaluation.MaxProviderInvocationsV1,
		Reason: "run the frozen v1 release evaluation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if starter.command.RunID != meta.ID(700) || starter.command.OrgID != 7 || starter.command.RequestedBy != "user:42" || committer.run == nil {
		t.Fatalf("start audit/commit = %#v / %#v", starter.command, committer.run)
	}

	_, err = service.StartEvaluation(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, StartEvaluationCommand{
		Confirm: true, ExpectedProviderInvocations: appevaluation.MaxProviderInvocationsV1 - 1, Reason: "stale cost acknowledgement",
	})
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrInvalidArgument {
		t.Fatalf("stale cost confirmation error = %v", err)
	}
}

func TestStartEvaluationMapsOrganizationAdmissionRejectionsToCapacityCode(t *testing.T) {
	for name, admissionErr := range map[string]error{
		"organization concurrency": domainevaluation.ErrOrgConcurrencyExceeded,
		"daily budget":             domainevaluation.ErrDailyBudgetExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			service := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{}, WithEvaluationExecution(
				&evaluationStarterStub{}, &evaluationStartCommitterStub{commitErr: admissionErr}, func() meta.ID { return meta.ID(700) },
			))
			_, err := service.StartEvaluation(context.Background(), Actor{OrgID: 7, OperatorUserID: 42}, StartEvaluationCommand{
				Confirm: true, ExpectedProviderInvocations: appevaluation.MaxProviderInvocationsV1,
				Reason: "run the frozen v1 release evaluation",
			})
			coder := cberrors.ParseCoder(err)
			if coder == nil || coder.Code() != code.ErrAIExplanationCapacityExceeded {
				t.Fatalf("capacity error = %v", err)
			}
		})
	}
}

func TestFindEvaluationCapacityUsesTrustedOrganizationAndCurrentUTCDay(t *testing.T) {
	now := time.Date(2026, 8, 28, 1, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	day := domainevaluation.UTCBudgetDay(now)
	reader := &capacityReaderStub{found: true, usage: domainevaluation.DailyCapacityUsage{
		OrgID: 7, BudgetDay: day, ReservedProviderInvocations: 70,
		Reservations: []domainevaluation.DailyCapacityUsageReservation{{
			RunID: meta.ID(700), RequestedBy: "user:42", ProviderInvocations: 70, ReservedAt: day.Add(time.Hour),
		}},
	}}
	access := &accessStub{}
	service := NewService(&reviewWorkflowStub{}, &governanceStub{}, access,
		WithEvaluationCapacity(reader, 1, 140, func() time.Time { return now }),
	)
	result, err := service.FindEvaluationCapacity(context.Background(), Actor{OrgID: 7, OperatorUserID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if reader.orgID != 7 || !reader.budgetDay.Equal(day) || access.governanceCalls != 1 {
		t.Fatalf("trusted capacity query = org:%d day:%s auth:%d", reader.orgID, reader.budgetDay, access.governanceCalls)
	}
	if result.DailyProviderInvocationLimit != 140 || result.ReservedProviderInvocations != 70 ||
		result.RemainingProviderInvocations != 70 || result.AvailableFullRunStarts != 1 || result.OverLimit ||
		len(result.Reservations) != 1 || result.Reservations[0].RunID != meta.ID(700) {
		t.Fatalf("capacity projection = %#v", result)
	}
}

func TestFindEvaluationCapacityFailsClosedWhenDisabledOrReaderCrossesOrganization(t *testing.T) {
	actor := Actor{OrgID: 7, OperatorUserID: 42}
	disabled := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{})
	_, err := disabled.FindEvaluationCapacity(context.Background(), actor)
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrUnsupportedOperation {
		t.Fatalf("disabled capacity error = %v", err)
	}

	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	reader := &capacityReaderStub{found: true, usage: domainevaluation.DailyCapacityUsage{OrgID: 8, BudgetDay: day}}
	configured := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{},
		WithEvaluationCapacity(reader, 1, 140, func() time.Time { return day.Add(time.Hour) }),
	)
	if _, err = configured.FindEvaluationCapacity(context.Background(), actor); err == nil {
		t.Fatal("expected cross-organization capacity projection to be rejected")
	}
}

func TestFindParticipantCapacityUsesTrustedOrganizationAndCurrentUTCDay(t *testing.T) {
	now := time.Date(2026, 8, 28, 1, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	day := domaingeneration.ParticipantUTCBudgetDay(now)
	reader := &participantCapacityReaderStub{found: true, usage: domaingeneration.ParticipantDailyCapacityUsage{
		OrgID: 7, BudgetDay: day, ReservedProviderInvocations: 1,
		Reservations: []domaingeneration.ParticipantDailyCapacityUsageReservation{{
			ReservationID: domaingeneration.ParticipantCapacityReservationID(meta.ID(900), 1),
			GenerationID:  meta.ID(900), Attempt: 1, Origin: retrygovernance.AttemptOriginInitial,
			UserID: "user-42", AssessmentID: meta.ID(501),
			ProviderInvocations: 1, ReservedAt: day.Add(time.Hour),
		}},
	}}
	activeReader := &participantActiveCapacityReaderStub{found: true, usage: domaingeneration.ParticipantActiveCapacityUsage{
		OrgID: 7, ActiveExecutions: 1,
		Reservations: []domaingeneration.ParticipantActiveCapacityUsageReservation{{
			GenerationID: meta.ID(900), RunID: meta.ID(901), UserID: "user-42", AssessmentID: meta.ID(501),
			AcquiredAt: day.Add(2 * time.Hour),
		}},
	}}
	policy := domaingeneration.ParticipantCapacityPolicy{
		DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
		DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
		MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
	}
	access := &accessStub{}
	service := NewService(&reviewWorkflowStub{}, &governanceStub{}, access,
		WithParticipantCapacity(reader, activeReader, policy, func() time.Time { return now }),
	)
	result, err := service.FindParticipantCapacity(context.Background(), Actor{OrgID: 7, OperatorUserID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if reader.orgID != 7 || !reader.budgetDay.Equal(day) || activeReader.orgID != 7 || access.governanceCalls != 1 {
		t.Fatalf("trusted participant capacity query = org:%d day:%s auth:%d", reader.orgID, reader.budgetDay, access.governanceCalls)
	}
	if result.DailyProviderInvocationLimitPerOrg != 500 || result.DailyProviderInvocationLimitPerUser != 5 ||
		result.DailyProviderInvocationLimitPerAssessment != 3 || result.ReservedProviderInvocations != 1 ||
		result.RemainingOrgProviderInvocations != 499 || result.OverOrgLimit || len(result.Reservations) != 1 ||
		result.Reservations[0].GenerationID != meta.ID(900) || result.MaxActiveProviderExecutionsPerOrg != 10 ||
		result.MaxActiveProviderExecutionsPerUser != 2 || result.MaxActiveProviderExecutionsPerAssessment != 1 ||
		result.ActiveProviderExecutions != 1 || result.RemainingOrgActiveProviderExecutions != 9 ||
		result.OverOrgActiveLimit || len(result.ActiveReservations) != 1 || result.ActiveReservations[0].RunID != meta.ID(901) {
		t.Fatalf("participant capacity projection = %#v", result)
	}
}

func TestFindParticipantCapacityFailsClosedWhenDisabledOrReaderCrossesOrganization(t *testing.T) {
	actor := Actor{OrgID: 7, OperatorUserID: 42}
	disabled := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{})
	_, err := disabled.FindParticipantCapacity(context.Background(), actor)
	if coder := cberrors.ParseCoder(err); coder == nil || coder.Code() != code.ErrUnsupportedOperation {
		t.Fatalf("disabled participant capacity error = %v", err)
	}

	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	reader := &participantCapacityReaderStub{found: true, usage: domaingeneration.ParticipantDailyCapacityUsage{OrgID: 8, BudgetDay: day}}
	policy := domaingeneration.ParticipantCapacityPolicy{
		DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
		DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
		MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
	}
	configured := NewService(&reviewWorkflowStub{}, &governanceStub{}, &accessStub{},
		WithParticipantCapacity(reader, &participantActiveCapacityReaderStub{}, policy, func() time.Time { return day.Add(time.Hour) }),
	)
	if _, err = configured.FindParticipantCapacity(context.Background(), actor); err == nil {
		t.Fatal("expected cross-organization participant capacity projection to be rejected")
	}
}

func TestRecoverAndCancelEvaluationKeepTrustedAudit(t *testing.T) {
	run := &appevaluation.ReviewRun{RunID: meta.ID(700), Status: domainevaluation.StatusCollecting, RequestedOrgID: 7, RecoveryMaxProviderInvocations: 70, CanCancel: true}
	reviews := &reviewWorkflowStub{run: run}
	committer := &evaluationStartCommitterStub{}
	service := NewService(reviews, &governanceStub{}, &accessStub{}, WithEvaluationExecution(
		&evaluationStarterStub{}, committer, func() meta.ID { return meta.ID(701) },
	))
	actor := Actor{OrgID: 7, OperatorUserID: 42}

	_, err := service.RecoverEvaluation(context.Background(), actor, meta.ID(700), RecoverEvaluationCommand{
		Confirm: true, ExpectedProviderInvocations: 70, Reason: "redeliver expired execution",
	})
	if err != nil {
		t.Fatal(err)
	}
	if committer.recoveryRequestID != "701" || committer.recoveryActor != "user:42" {
		t.Fatalf("recovery audit = %q/%q", committer.recoveryRequestID, committer.recoveryActor)
	}

	_, err = service.CancelEvaluation(context.Background(), actor, meta.ID(700), "stop before dispatch")
	if err != nil {
		t.Fatal(err)
	}
	if reviews.canceledBy != "user:42" || reviews.cancelReason != "stop before dispatch" {
		t.Fatalf("cancel audit = %q/%q", reviews.canceledBy, reviews.cancelReason)
	}
}

func TestRetryParticipantGenerationRequiresExactCostAndUsesTrustedAudit(t *testing.T) {
	recovery := &participantRecoveryStub{}
	access := &accessStub{}
	service := NewService(&reviewWorkflowStub{}, &governanceStub{}, access, WithParticipantRecovery(recovery))
	actor := Actor{OrgID: 7, OperatorUserID: 42}
	command := RetryParticipantGenerationCommand{
		GenerationID: meta.ID(900), ExpectedAttempt: 1, RequestID: "retry-request-1",
		Confirm: true, ExpectedProviderInvocations: domaingeneration.ParticipantProviderInvocationsPerAttemptV1,
		AcceptResultUnknownRisk: true, Reason: "manual recovery",
	}
	if _, err := service.RetryParticipantGeneration(context.Background(), actor, command); err != nil {
		t.Fatal(err)
	}
	if access.governanceCalls != 1 || recovery.calls != 1 || recovery.command.OrgID != 7 || recovery.command.Actor != "user:42" ||
		recovery.command.GenerationID != meta.ID(900) || recovery.command.RequestID != "retry-request-1" || !recovery.command.AcceptResultUnknownRisk {
		t.Fatalf("trusted participant retry = auth:%d calls:%d command:%#v", access.governanceCalls, recovery.calls, recovery.command)
	}

	command.ExpectedProviderInvocations = 2
	if _, err := service.RetryParticipantGeneration(context.Background(), actor, command); err == nil || cberrors.ParseCoder(err) == nil || cberrors.ParseCoder(err).Code() != code.ErrInvalidArgument {
		t.Fatalf("stale cost confirmation error = %v", err)
	}
	if recovery.calls != 1 {
		t.Fatalf("invalid cost reached recovery service; calls = %d", recovery.calls)
	}
}

func reviewRunFixture() *appevaluation.ReviewRun {
	return &appevaluation.ReviewRun{
		RunID: meta.ID(9), Status: domainevaluation.StatusAwaitingReview, CanReview: true,
		Attempts: []appevaluation.ReviewAttempt{{
			CaseID: "PROMPT-EVAL-001", Attempt: 1,
			MissingRoles: []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct},
		}},
	}
}

func profileDefinitionFixture(t *testing.T) domainprofile.Definition {
	t.Helper()
	suite, err := appevaluation.LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	return suite.ProfileFixture.Definition
}

type reviewWorkflowStub struct {
	run          *appevaluation.ReviewRun
	recorded     appevaluation.HumanReviewCommand
	recordCalls  int
	canceledBy   string
	cancelReason string
	listPage     *appevaluation.ReviewRunPage
	listQuery    appevaluation.ReviewRunListQuery
	listErr      error
}

func (s *reviewWorkflowStub) Find(context.Context, meta.ID) (*appevaluation.ReviewRun, error) {
	return s.run, nil
}
func (s *reviewWorkflowStub) RecordHumanReview(_ context.Context, _ meta.ID, command appevaluation.HumanReviewCommand) (*appevaluation.ReviewRun, error) {
	s.recorded, s.recordCalls = command, s.recordCalls+1
	return s.run, nil
}
func (s *reviewWorkflowStub) Finalize(context.Context, meta.ID, string, string) (*appevaluation.ReviewRun, error) {
	return s.run, nil
}
func (s *reviewWorkflowStub) Cancel(_ context.Context, _ meta.ID, actor, reason string) (*appevaluation.ReviewRun, error) {
	s.canceledBy, s.cancelReason = actor, reason
	return s.run, nil
}
func (s *reviewWorkflowStub) List(_ context.Context, query appevaluation.ReviewRunListQuery) (*appevaluation.ReviewRunPage, error) {
	s.listQuery = query
	return s.listPage, s.listErr
}

type governanceStub struct {
	created        appgovernance.CreateDraftCommand
	profile        *domainprofile.AIExplanationProfile
	listPage       *appgovernance.ProfilePage
	listQuery      appgovernance.ProfileListQuery
	foundProfileID string
	foundVersion   string
	listErr        error
}

func (s *governanceStub) CreateDraft(_ context.Context, command appgovernance.CreateDraftCommand) (*domainprofile.AIExplanationProfile, error) {
	s.created = command
	return domainprofile.NewDraftForRelease(meta.ID(1), command.Definition, command.Actor, command.Reason, time.Now())
}
func (*governanceStub) Publish(context.Context, appgovernance.PublishCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
func (*governanceStub) Disable(context.Context, appgovernance.DisableCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
func (s *governanceStub) List(_ context.Context, query appgovernance.ProfileListQuery) (*appgovernance.ProfilePage, error) {
	s.listQuery = query
	return s.listPage, s.listErr
}
func (s *governanceStub) Find(_ context.Context, profileID, version string) (*domainprofile.AIExplanationProfile, error) {
	s.foundProfileID, s.foundVersion = profileID, version
	return s.profile, nil
}

type accessStub struct {
	reviewRole      domainevaluation.ReviewRole
	governanceCalls int
	readCalls       int
	readActor       Actor
}

func (s *accessStub) AuthorizeRead(_ context.Context, actor Actor) error {
	s.readCalls++
	s.readActor = actor
	return nil
}
func (s *accessStub) AuthorizeReview(_ context.Context, _ Actor, role domainevaluation.ReviewRole) error {
	s.reviewRole = role
	return nil
}
func (s *accessStub) AuthorizeGovernance(context.Context, Actor) error {
	s.governanceCalls++
	return nil
}

type capacityReaderStub struct {
	usage     domainevaluation.DailyCapacityUsage
	found     bool
	err       error
	orgID     int64
	budgetDay time.Time
}

type participantCapacityReaderStub struct {
	usage     domaingeneration.ParticipantDailyCapacityUsage
	found     bool
	err       error
	orgID     int64
	budgetDay time.Time
}

type participantActiveCapacityReaderStub struct {
	usage domaingeneration.ParticipantActiveCapacityUsage
	found bool
	err   error
	orgID int64
}

type participantRecoveryStub struct {
	command apprecovery.Command
	calls   int
}

func (s *participantRecoveryStub) Authorize(_ context.Context, command apprecovery.Command) (*apprecovery.Result, error) {
	s.command, s.calls = command, s.calls+1
	return nil, nil
}

func (s *participantCapacityReaderStub) FindParticipantDailyCapacityUsage(_ context.Context, orgID int64, budgetDay time.Time) (domaingeneration.ParticipantDailyCapacityUsage, bool, error) {
	s.orgID, s.budgetDay = orgID, budgetDay
	return s.usage, s.found, s.err
}

func (s *participantActiveCapacityReaderStub) FindParticipantActiveCapacityUsage(_ context.Context, orgID int64) (domaingeneration.ParticipantActiveCapacityUsage, bool, error) {
	s.orgID = orgID
	return s.usage, s.found, s.err
}

func (s *capacityReaderStub) FindDailyCapacityUsage(_ context.Context, orgID int64, budgetDay time.Time) (domainevaluation.DailyCapacityUsage, bool, error) {
	s.orgID, s.budgetDay = orgID, budgetDay
	return s.usage, s.found, s.err
}

type evaluationStarterStub struct {
	command appevaluation.OnlineStartCommand
}

func (s *evaluationStarterStub) PrepareRequestedV1(_ context.Context, command appevaluation.OnlineStartCommand) (*appevaluation.OnlineRunResult, error) {
	s.command = command
	runRecord, err := domainevaluation.NewRequested(command.RunID, administrationEvaluationRelease(), command.OrgID, command.RequestedBy, command.Reason, time.Now())
	if err != nil {
		return nil, err
	}
	return &appevaluation.OnlineRunResult{Run: runRecord}, nil
}

type evaluationStartCommitterStub struct {
	run               *domainevaluation.PromptEvaluationRun
	commitErr         error
	recoveryRequestID string
	recoveryActor     string
}

func (s *evaluationStartCommitterStub) CommitStart(_ context.Context, runRecord *domainevaluation.PromptEvaluationRun) error {
	s.run = runRecord
	return s.commitErr
}

func (s *evaluationStartCommitterStub) CommitRecovery(_ context.Context, _ meta.ID, requestID, actor, _ string) (*domainevaluation.PromptEvaluationRun, error) {
	s.recoveryRequestID, s.recoveryActor = requestID, actor
	return s.run, nil
}

func administrationEvaluationRelease() domainevaluation.ReleaseIdentity {
	return domainevaluation.ReleaseIdentity{
		Suite:        domainevaluation.SuiteRef{ID: "suite-v1", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("suite")), GitBlobSHA: "suite-blob"},
		Prompt:       aiexplanation.PromptRef{TemplateID: "prompt", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "prompt-blob"},
		Profile:      aiexplanation.ProfileRef{ID: "profile", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("profile"))},
		InputSchema:  domainevaluation.SchemaRef{Version: aiexplanation.InputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("input"))},
		OutputSchema: domainevaluation.SchemaRef{Version: aiexplanation.OutputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("output"))},
		Provider:     aiexplanation.ProviderExecutionSpec{Route: "route", RouteRevision: "v1", ResolvedProvider: "provider", ResolvedModel: "model", Fingerprint: aiexplanation.NewFingerprint([]byte("route"))},
		Decoding:     domainevaluation.DecodingParameters{MaxOutputTokens: 1000}, SemanticEvaluator: domainevaluation.SemanticEvaluatorSpec{
			Version: "judge-v1", Prompt: aiexplanation.PromptRef{TemplateID: "judge", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("judge")), GitBlobSHA: "judge-blob"},
			OutputSchema: domainevaluation.SchemaRef{Version: "judge-output/v1", Fingerprint: aiexplanation.NewFingerprint([]byte("judge-output"))},
			Provider:     aiexplanation.ProviderExecutionSpec{Route: "judge", RouteRevision: "v1", ResolvedProvider: "provider", ResolvedModel: "judge-model", Fingerprint: aiexplanation.NewFingerprint([]byte("judge-route"))},
			Decoding:     domainevaluation.DecodingParameters{MaxOutputTokens: 1000},
		},
		GenerationCaseIDs: []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"},
		PreflightCaseID:   "p1", PreflightRejectionReason: "ineligible", RepetitionsPerCase: 5,
	}
}
