// Package administration exposes the operator-only review and release
// workflow for AI explanation assets. It never invokes a Provider and never
// serves participant content.
package administration

import (
	"context"
	stderrors "errors"
	"strconv"
	"strings"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appgovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	apprecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

const maxAuditReasonLength = 1000

type Actor struct {
	OrgID          int64
	OperatorUserID int64
}

func (a Actor) Subject() string {
	if a.OperatorUserID <= 0 {
		return ""
	}
	return authz.SubjectKey(strconv.FormatInt(a.OperatorUserID, 10))
}

type Access interface {
	AuthorizeRead(context.Context, Actor) error
	AuthorizeReview(context.Context, Actor, domainevaluation.ReviewRole) error
	AuthorizeGovernance(context.Context, Actor) error
}

type ReviewWorkflow interface {
	List(context.Context, appevaluation.ReviewRunListQuery) (*appevaluation.ReviewRunPage, error)
	Find(context.Context, meta.ID) (*appevaluation.ReviewRun, error)
	RecordHumanReview(context.Context, meta.ID, appevaluation.HumanReviewCommand) (*appevaluation.ReviewRun, error)
	Finalize(context.Context, meta.ID, string, string) (*appevaluation.ReviewRun, error)
	Cancel(context.Context, meta.ID, string, string) (*appevaluation.ReviewRun, error)
}

type ProfileGovernance interface {
	List(context.Context, appgovernance.ProfileListQuery) (*appgovernance.ProfilePage, error)
	Find(context.Context, string, string) (*domainprofile.AIExplanationProfile, error)
	CreateDraft(context.Context, appgovernance.CreateDraftCommand) (*domainprofile.AIExplanationProfile, error)
	Publish(context.Context, appgovernance.PublishCommand) (*domainprofile.AIExplanationProfile, error)
	Disable(context.Context, appgovernance.DisableCommand) (*domainprofile.AIExplanationProfile, error)
}

type EvaluationStarter interface {
	PrepareRequestedV1(context.Context, appevaluation.OnlineStartCommand) (*appevaluation.OnlineRunResult, error)
}

type EvaluationStartCommitter interface {
	CommitStart(context.Context, *domainevaluation.PromptEvaluationRun) error
}

type EvaluationExecutionCommitter interface {
	EvaluationStartCommitter
	CommitRecovery(context.Context, meta.ID, string, string, string) (*domainevaluation.PromptEvaluationRun, error)
}

type Service interface {
	FindEvaluationCapacity(context.Context, Actor) (*EvaluationCapacity, error)
	FindParticipantCapacity(context.Context, Actor) (*ParticipantCapacity, error)
	RetryParticipantGeneration(context.Context, Actor, RetryParticipantGenerationCommand) (*apprecovery.Result, error)
	ListEvaluations(context.Context, Actor, EvaluationListQuery) (*appevaluation.ReviewRunPage, error)
	FindEvaluation(context.Context, Actor, meta.ID) (*appevaluation.ReviewRun, error)
	StartEvaluation(context.Context, Actor, StartEvaluationCommand) (*appevaluation.ReviewRun, error)
	RecoverEvaluation(context.Context, Actor, meta.ID, RecoverEvaluationCommand) (*appevaluation.ReviewRun, error)
	CancelEvaluation(context.Context, Actor, meta.ID, string) (*appevaluation.ReviewRun, error)
	RecordReview(context.Context, Actor, meta.ID, ReviewCommand) (*appevaluation.ReviewRun, error)
	FinalizeEvaluation(context.Context, Actor, meta.ID, string) (*appevaluation.ReviewRun, error)
	ListProfiles(context.Context, Actor, ProfileListQuery) (*appgovernance.ProfilePage, error)
	FindProfile(context.Context, Actor, string, string) (*domainprofile.AIExplanationProfile, error)
	CreateProfileDraft(context.Context, Actor, CreateProfileDraftCommand) (*domainprofile.AIExplanationProfile, error)
	PublishProfile(context.Context, Actor, PublishProfileCommand) (*domainprofile.AIExplanationProfile, error)
	DisableProfile(context.Context, Actor, DisableProfileCommand) (*domainprofile.AIExplanationProfile, error)
}

type EvaluationListQuery struct {
	Status *domainevaluation.Status
	Cursor string
	Limit  int
}

type ProfileListQuery struct {
	Status *domainprofile.Status
	Cursor string
	Limit  int
}

type ReviewCommand struct {
	CaseID   string
	Attempt  int
	Role     domainevaluation.ReviewRole
	Decision domainevaluation.ReviewDecision
	Reason   string
}

type StartEvaluationCommand struct {
	Confirm                     bool
	ExpectedProviderInvocations int
	Reason                      string
}

type RecoverEvaluationCommand struct {
	Confirm                     bool
	ExpectedProviderInvocations int
	Reason                      string
}

type RetryParticipantGenerationCommand struct {
	GenerationID                meta.ID
	ExpectedAttempt             int
	RequestID                   string
	Confirm                     bool
	ExpectedProviderInvocations int
	AcceptResultUnknownRisk     bool
	Reason                      string
}

type EvaluationCapacity struct {
	OrgID                        int64
	BudgetDay                    time.Time
	MaxActiveRunsPerOrg          int
	ProviderInvocationsPerStart  int
	DailyProviderInvocationLimit int
	ReservedProviderInvocations  int
	RemainingProviderInvocations int
	AvailableFullRunStarts       int
	OverLimit                    bool
	Reservations                 []EvaluationCapacityReservation
}

type EvaluationCapacityReservation struct {
	RunID               meta.ID
	RequestedBy         string
	ProviderInvocations int
	ReservedAt          time.Time
}

type ParticipantCapacity struct {
	OrgID                                     int64
	BudgetDay                                 time.Time
	ProviderInvocationsPerGeneration          int
	DailyProviderInvocationLimitPerOrg        int
	DailyProviderInvocationLimitPerUser       int
	DailyProviderInvocationLimitPerAssessment int
	MaxActiveProviderExecutionsPerOrg         int
	MaxActiveProviderExecutionsPerUser        int
	MaxActiveProviderExecutionsPerAssessment  int
	ReservedProviderInvocations               int
	RedactedProviderInvocations               int
	RemainingOrgProviderInvocations           int
	OverOrgLimit                              bool
	Reservations                              []ParticipantCapacityReservation
	ActiveProviderExecutions                  int
	RemainingOrgActiveProviderExecutions      int
	OverOrgActiveLimit                        bool
	ActiveReservations                        []ParticipantActiveCapacityReservation
}

type ParticipantCapacityReservation struct {
	ReservationID       string
	GenerationID        meta.ID
	Attempt             int
	Origin              retrygovernance.AttemptOrigin
	UserID              string
	AssessmentID        meta.ID
	ProviderInvocations int
	ReservedAt          time.Time
}

type ParticipantActiveCapacityReservation struct {
	GenerationID meta.ID
	RunID        meta.ID
	UserID       string
	AssessmentID meta.ID
	AcquiredAt   time.Time
}

type CreateProfileDraftCommand struct {
	Definition          domainprofile.Definition
	ExpectedFingerprint aiexplanation.Fingerprint
	Reason              string
}

type PublishProfileCommand struct {
	ProfileID       string
	ProfileVersion  string
	EvaluationRunID meta.ID
	Reason          string
}

type DisableProfileCommand struct {
	ProfileID      string
	ProfileVersion string
	Reason         string
}

type service struct {
	reviews                       ReviewWorkflow
	governance                    ProfileGovernance
	access                        Access
	starter                       EvaluationStarter
	startCommitter                EvaluationExecutionCommitter
	capacity                      domainevaluation.CapacityReader
	participantCapacity           domaingeneration.ParticipantCapacityReader
	participantActiveCapacity     domaingeneration.ParticipantActiveCapacityReader
	participantCapacityPolicy     domaingeneration.ParticipantCapacityPolicy
	participantRecovery           apprecovery.Service
	maxActiveRunsPerOrg           int
	dailyProviderInvocationBudget int
	newID                         func() meta.ID
	now                           func() time.Time
}

type Option func(*service)

func WithEvaluationExecution(starter EvaluationStarter, committer EvaluationExecutionCommitter, newID func() meta.ID) Option {
	return func(value *service) {
		value.starter = starter
		value.startCommitter = committer
		if newID != nil {
			value.newID = newID
		}
	}
}

func WithEvaluationCapacity(reader domainevaluation.CapacityReader, maxActiveRunsPerOrg, dailyProviderInvocationBudget int, now func() time.Time) Option {
	return func(value *service) {
		if reader == nil || maxActiveRunsPerOrg != 1 || dailyProviderInvocationBudget < appevaluation.MaxProviderInvocationsV1 ||
			dailyProviderInvocationBudget%appevaluation.MaxProviderInvocationsV1 != 0 {
			return
		}
		value.capacity = reader
		value.maxActiveRunsPerOrg = maxActiveRunsPerOrg
		value.dailyProviderInvocationBudget = dailyProviderInvocationBudget
		if now != nil {
			value.now = now
		}
	}
}

func WithParticipantCapacity(reader domaingeneration.ParticipantCapacityReader, activeReader domaingeneration.ParticipantActiveCapacityReader, policy domaingeneration.ParticipantCapacityPolicy, now func() time.Time) Option {
	return func(value *service) {
		if reader == nil || activeReader == nil || policy.Validate() != nil {
			return
		}
		value.participantCapacity = reader
		value.participantActiveCapacity = activeReader
		value.participantCapacityPolicy = policy
		if now != nil {
			value.now = now
		}
	}
}

func WithParticipantRecovery(recovery apprecovery.Service) Option {
	return func(value *service) {
		value.participantRecovery = recovery
	}
}

func (s *service) RetryParticipantGeneration(ctx context.Context, actor Actor, command RetryParticipantGenerationCommand) (*apprecovery.Result, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || command.GenerationID.IsZero() || command.ExpectedAttempt < 1 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation participant recovery identity is invalid")
	}
	command.RequestID = strings.TrimSpace(command.RequestID)
	command.Reason = strings.TrimSpace(command.Reason)
	if !command.Confirm || command.ExpectedProviderInvocations != domaingeneration.ParticipantProviderInvocationsPerAttemptV1 ||
		command.RequestID == "" || len(command.RequestID) > 256 || command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation participant recovery confirmation is invalid")
	}
	if s == nil || s.participantRecovery == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation participant recovery is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.participantRecovery.Authorize(ctx, apprecovery.Command{
		OrgID: actor.OrgID, GenerationID: command.GenerationID, ExpectedAttempt: command.ExpectedAttempt,
		RequestID: command.RequestID, Actor: actor.Subject(), Reason: command.Reason,
		AcceptResultUnknownRisk: command.AcceptResultUnknownRisk,
	})
	return result, mapKnownError(err)
}

func (s *service) RecoverEvaluation(ctx context.Context, actor Actor, runID meta.ID, command RecoverEvaluationCommand) (*appevaluation.ReviewRun, error) {
	if err := s.ensureReviewConfigured(actor, runID); err != nil {
		return nil, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if !command.Confirm || command.ExpectedProviderInvocations <= 0 || command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation recovery cost confirmation is invalid")
	}
	if s.startCommitter == nil || s.newID == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation online evaluation is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	current, err := s.reviews.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationOrg(current, actor); err != nil {
		return nil, err
	}
	if current.RecoveryMaxProviderInvocations <= 0 || command.ExpectedProviderInvocations != current.RecoveryMaxProviderInvocations {
		return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation recovery cost confirmation is stale")
	}
	if _, err := s.startCommitter.CommitRecovery(ctx, runID, s.newID().String(), actor.Subject(), command.Reason); err != nil {
		return nil, mapKnownError(err)
	}
	result, err := s.reviews.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationOrg(result, actor); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *service) CancelEvaluation(ctx context.Context, actor Actor, runID meta.ID, reason string) (*appevaluation.ReviewRun, error) {
	if err := s.ensureReviewConfigured(actor, runID); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation cancellation reason is invalid")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	current, err := s.reviews.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationOrg(current, actor); err != nil {
		return nil, err
	}
	result, err := s.reviews.Cancel(ctx, runID, actor.Subject(), reason)
	return result, mapKnownError(err)
}

func NewService(reviews ReviewWorkflow, governance ProfileGovernance, access Access, options ...Option) Service {
	value := &service{reviews: reviews, governance: governance, access: access, newID: meta.New, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(value)
		}
	}
	return value
}

func (s *service) FindEvaluationCapacity(ctx context.Context, actor Actor) (*EvaluationCapacity, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation administrator identity is required")
	}
	if s == nil || s.capacity == nil || s.access == nil || s.now == nil || s.maxActiveRunsPerOrg != 1 ||
		s.dailyProviderInvocationBudget < appevaluation.MaxProviderInvocationsV1 {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation evaluation capacity is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	budgetDay := domainevaluation.UTCBudgetDay(s.now())
	usage, found, err := s.capacity.FindDailyCapacityUsage(ctx, actor.OrgID, budgetDay)
	if err != nil {
		return nil, err
	}
	if !found {
		usage = domainevaluation.DailyCapacityUsage{OrgID: actor.OrgID, BudgetDay: budgetDay}
	}
	if err := usage.Validate(); err != nil {
		return nil, err
	}
	if usage.OrgID != actor.OrgID || !usage.BudgetDay.Equal(budgetDay) {
		return nil, cberrors.WithCode(code.ErrUnknown, "AI explanation evaluation capacity identity is inconsistent")
	}
	remaining := s.dailyProviderInvocationBudget - usage.ReservedProviderInvocations
	if remaining < 0 {
		remaining = 0
	}
	result := &EvaluationCapacity{
		OrgID: actor.OrgID, BudgetDay: budgetDay, MaxActiveRunsPerOrg: s.maxActiveRunsPerOrg,
		ProviderInvocationsPerStart:  appevaluation.MaxProviderInvocationsV1,
		DailyProviderInvocationLimit: s.dailyProviderInvocationBudget,
		ReservedProviderInvocations:  usage.ReservedProviderInvocations,
		RemainingProviderInvocations: remaining,
		AvailableFullRunStarts:       remaining / appevaluation.MaxProviderInvocationsV1,
		OverLimit:                    usage.ReservedProviderInvocations > s.dailyProviderInvocationBudget,
		Reservations:                 make([]EvaluationCapacityReservation, 0, len(usage.Reservations)),
	}
	for _, reservation := range usage.Reservations {
		result.Reservations = append(result.Reservations, EvaluationCapacityReservation{
			RunID: reservation.RunID, RequestedBy: reservation.RequestedBy,
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt,
		})
	}
	return result, nil
}

func (s *service) FindParticipantCapacity(ctx context.Context, actor Actor) (*ParticipantCapacity, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation administrator identity is required")
	}
	if s == nil || s.participantCapacity == nil || s.participantActiveCapacity == nil || s.access == nil || s.now == nil || s.participantCapacityPolicy.Validate() != nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation participant capacity is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	budgetDay := domaingeneration.ParticipantUTCBudgetDay(s.now())
	usage, found, err := s.participantCapacity.FindParticipantDailyCapacityUsage(ctx, actor.OrgID, budgetDay)
	if err != nil {
		return nil, err
	}
	if !found {
		usage = domaingeneration.ParticipantDailyCapacityUsage{OrgID: actor.OrgID, BudgetDay: budgetDay}
	}
	if err := usage.Validate(); err != nil {
		return nil, err
	}
	if usage.OrgID != actor.OrgID || !usage.BudgetDay.Equal(budgetDay) {
		return nil, cberrors.WithCode(code.ErrUnknown, "AI explanation participant capacity identity is inconsistent")
	}
	activeUsage, activeFound, err := s.participantActiveCapacity.FindParticipantActiveCapacityUsage(ctx, actor.OrgID)
	if err != nil {
		return nil, err
	}
	if !activeFound {
		activeUsage = domaingeneration.ParticipantActiveCapacityUsage{OrgID: actor.OrgID}
	}
	if err := activeUsage.Validate(); err != nil {
		return nil, err
	}
	if activeUsage.OrgID != actor.OrgID {
		return nil, cberrors.WithCode(code.ErrUnknown, "AI explanation participant active capacity identity is inconsistent")
	}
	policy := s.participantCapacityPolicy
	remaining := policy.DailyProviderInvocationBudgetPerOrg - usage.ReservedProviderInvocations
	if remaining < 0 {
		remaining = 0
	}
	remainingActive := policy.MaxActiveProviderExecutionsPerOrg - activeUsage.ActiveExecutions
	if remainingActive < 0 {
		remainingActive = 0
	}
	result := &ParticipantCapacity{
		OrgID: actor.OrgID, BudgetDay: budgetDay,
		ProviderInvocationsPerGeneration:          domaingeneration.ParticipantProviderInvocationsPerGenerationV1,
		DailyProviderInvocationLimitPerOrg:        policy.DailyProviderInvocationBudgetPerOrg,
		DailyProviderInvocationLimitPerUser:       policy.DailyProviderInvocationBudgetPerUser,
		DailyProviderInvocationLimitPerAssessment: policy.DailyProviderInvocationBudgetPerAssessment,
		MaxActiveProviderExecutionsPerOrg:         policy.MaxActiveProviderExecutionsPerOrg,
		MaxActiveProviderExecutionsPerUser:        policy.MaxActiveProviderExecutionsPerUser,
		MaxActiveProviderExecutionsPerAssessment:  policy.MaxActiveProviderExecutionsPerAssessment,
		ReservedProviderInvocations:               usage.ReservedProviderInvocations,
		RedactedProviderInvocations:               usage.RedactedProviderInvocations,
		RemainingOrgProviderInvocations:           remaining,
		OverOrgLimit:                              usage.ReservedProviderInvocations > policy.DailyProviderInvocationBudgetPerOrg,
		Reservations:                              make([]ParticipantCapacityReservation, 0, len(usage.Reservations)),
		ActiveProviderExecutions:                  activeUsage.ActiveExecutions,
		RemainingOrgActiveProviderExecutions:      remainingActive,
		OverOrgActiveLimit:                        activeUsage.ActiveExecutions > policy.MaxActiveProviderExecutionsPerOrg,
		ActiveReservations:                        make([]ParticipantActiveCapacityReservation, 0, len(activeUsage.Reservations)),
	}
	for _, reservation := range usage.Reservations {
		result.Reservations = append(result.Reservations, ParticipantCapacityReservation{
			ReservationID: reservation.ReservationID, GenerationID: reservation.GenerationID,
			Attempt: reservation.Attempt, Origin: reservation.Origin,
			UserID: reservation.UserID, AssessmentID: reservation.AssessmentID,
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt,
		})
	}
	for _, reservation := range activeUsage.Reservations {
		result.ActiveReservations = append(result.ActiveReservations, ParticipantActiveCapacityReservation{
			GenerationID: reservation.GenerationID, RunID: reservation.RunID, UserID: reservation.UserID,
			AssessmentID: reservation.AssessmentID, AcquiredAt: reservation.AcquiredAt,
		})
	}
	return result, nil
}

func (s *service) FindEvaluation(ctx context.Context, actor Actor, runID meta.ID) (*appevaluation.ReviewRun, error) {
	if err := s.ensureReviewConfigured(actor, runID); err != nil {
		return nil, err
	}
	if err := s.access.AuthorizeRead(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.reviews.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationOrg(result, actor); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *service) ListEvaluations(ctx context.Context, actor Actor, query EvaluationListQuery) (*appevaluation.ReviewRunPage, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || query.Limit < 0 || query.Limit > appevaluation.MaxReviewRunPageSize ||
		(query.Status != nil && !query.Status.IsValid()) {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation list query is invalid")
	}
	if s == nil || s.reviews == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation evaluation review is disabled")
	}
	if err := s.access.AuthorizeRead(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.reviews.List(ctx, appevaluation.ReviewRunListQuery{
		OrgID: actor.OrgID, Status: query.Status, Cursor: strings.TrimSpace(query.Cursor), Limit: query.Limit,
	})
	return result, mapKnownError(err)
}

func (s *service) ListProfiles(ctx context.Context, actor Actor, query ProfileListQuery) (*appgovernance.ProfilePage, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || query.Limit < 0 || query.Limit > appgovernance.MaxProfilePageSize ||
		(query.Status != nil && !query.Status.IsValid()) {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation Profile list query is invalid")
	}
	if s == nil || s.governance == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation Profile governance is disabled")
	}
	if err := s.access.AuthorizeRead(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.governance.List(ctx, appgovernance.ProfileListQuery{
		Status: query.Status, Cursor: strings.TrimSpace(query.Cursor), Limit: query.Limit,
	})
	return result, mapKnownError(err)
}

func (s *service) FindProfile(ctx context.Context, actor Actor, profileID, version string) (*domainprofile.AIExplanationProfile, error) {
	profileID, version = strings.TrimSpace(profileID), strings.TrimSpace(version)
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || profileID == "" || aiexplanation.ValidateVersion(version) != nil {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation Profile query is invalid")
	}
	if s == nil || s.governance == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation Profile governance is disabled")
	}
	if err := s.access.AuthorizeRead(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.governance.Find(ctx, profileID, version)
	return result, mapKnownError(err)
}

func (s *service) StartEvaluation(ctx context.Context, actor Actor, command StartEvaluationCommand) (*appevaluation.ReviewRun, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation administrator identity is required")
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if !command.Confirm || command.ExpectedProviderInvocations != appevaluation.MaxProviderInvocationsV1 ||
		command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation cost confirmation is invalid")
	}
	if s == nil || s.starter == nil || s.startCommitter == nil || s.reviews == nil || s.access == nil || s.newID == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "AI explanation online evaluation is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	prepared, err := s.starter.PrepareRequestedV1(ctx, appevaluation.OnlineStartCommand{
		RunID: s.newID(), OrgID: actor.OrgID, RequestedBy: actor.Subject(), Reason: command.Reason,
	})
	if err != nil {
		return nil, mapKnownError(err)
	}
	if prepared == nil || prepared.Run == nil {
		return nil, cberrors.WithCode(code.ErrUnknown, "AI explanation evaluation preparation returned no run")
	}
	if err := s.startCommitter.CommitStart(ctx, prepared.Run); err != nil {
		return nil, mapKnownError(err)
	}
	result, err := s.reviews.Find(ctx, prepared.Run.ID())
	return result, mapKnownError(err)
}

func (s *service) RecordReview(ctx context.Context, actor Actor, runID meta.ID, command ReviewCommand) (*appevaluation.ReviewRun, error) {
	if err := s.ensureReviewConfigured(actor, runID); err != nil {
		return nil, err
	}
	command.CaseID = strings.TrimSpace(command.CaseID)
	command.Reason = strings.TrimSpace(command.Reason)
	if command.CaseID == "" || command.Attempt < 1 || !validRole(command.Role) || !validDecision(command.Decision) || command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation review command is invalid")
	}
	if err := s.access.AuthorizeReview(ctx, actor, command.Role); err != nil {
		return nil, err
	}
	current, err := s.reviews.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationOrg(current, actor); err != nil {
		return nil, err
	}
	if err := validateReviewTarget(current, actor.Subject(), command); err != nil {
		return nil, err
	}
	result, err := s.reviews.RecordHumanReview(ctx, runID, appevaluation.HumanReviewCommand{
		CaseID: command.CaseID, Attempt: command.Attempt, Role: command.Role,
		Reviewer: actor.Subject(), Decision: command.Decision, Reason: command.Reason,
	})
	return result, mapKnownError(err)
}

func (s *service) FinalizeEvaluation(ctx context.Context, actor Actor, runID meta.ID, reason string) (*appevaluation.ReviewRun, error) {
	if err := s.ensureReviewConfigured(actor, runID); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation finalization reason is invalid")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	current, err := s.reviews.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationOrg(current, actor); err != nil {
		return nil, err
	}
	if !current.CanFinalize {
		return nil, cberrors.WithCode(code.ErrConflict, "AI explanation evaluation is not ready to finalize")
	}
	result, err := s.reviews.Finalize(ctx, runID, actor.Subject(), reason)
	return result, mapKnownError(err)
}

func (s *service) CreateProfileDraft(ctx context.Context, actor Actor, command CreateProfileDraftCommand) (*domainprofile.AIExplanationProfile, error) {
	if err := s.ensureGovernanceConfigured(actor); err != nil {
		return nil, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.Reason == "" || len(command.Reason) > maxAuditReasonLength || command.Definition.Validate() != nil || command.ExpectedFingerprint.Validate() != nil {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation Profile draft command is invalid")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.governance.CreateDraft(ctx, appgovernance.CreateDraftCommand{
		Definition: command.Definition, ExpectedFingerprint: command.ExpectedFingerprint,
		Actor: actor.Subject(), Reason: command.Reason,
	})
	return result, mapKnownError(err)
}

func (s *service) PublishProfile(ctx context.Context, actor Actor, command PublishProfileCommand) (*domainprofile.AIExplanationProfile, error) {
	if err := s.ensureGovernanceConfigured(actor); err != nil {
		return nil, err
	}
	command.ProfileID = strings.TrimSpace(command.ProfileID)
	command.ProfileVersion = strings.TrimSpace(command.ProfileVersion)
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ProfileID == "" || aiexplanation.ValidateVersion(command.ProfileVersion) != nil || command.EvaluationRunID.IsZero() || command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation Profile publish command is invalid")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.governance.Publish(ctx, appgovernance.PublishCommand{
		ProfileID: command.ProfileID, ProfileVersion: command.ProfileVersion,
		EvaluationRunID: command.EvaluationRunID, Actor: actor.Subject(), Reason: command.Reason,
	})
	return result, mapKnownError(err)
}

func (s *service) DisableProfile(ctx context.Context, actor Actor, command DisableProfileCommand) (*domainprofile.AIExplanationProfile, error) {
	if err := s.ensureGovernanceConfigured(actor); err != nil {
		return nil, err
	}
	command.ProfileID = strings.TrimSpace(command.ProfileID)
	command.ProfileVersion = strings.TrimSpace(command.ProfileVersion)
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ProfileID == "" || aiexplanation.ValidateVersion(command.ProfileVersion) != nil || command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation Profile disable command is invalid")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	result, err := s.governance.Disable(ctx, appgovernance.DisableCommand{
		ProfileID: command.ProfileID, ProfileVersion: command.ProfileVersion,
		Actor: actor.Subject(), Reason: command.Reason,
	})
	return result, mapKnownError(err)
}

func (s *service) ensureReviewConfigured(actor Actor, runID meta.ID) error {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || runID.IsZero() {
		return cberrors.WithCode(code.ErrInvalidArgument, "AI explanation administrator identity and evaluation run are required")
	}
	if s == nil || s.reviews == nil || s.access == nil {
		return cberrors.WithCode(code.ErrModuleInitializationFailed, "AI explanation review administration is not configured")
	}
	return nil
}

func (s *service) ensureGovernanceConfigured(actor Actor) error {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return cberrors.WithCode(code.ErrInvalidArgument, "AI explanation administrator identity is required")
	}
	if s == nil || s.governance == nil || s.access == nil {
		return cberrors.WithCode(code.ErrModuleInitializationFailed, "AI explanation Profile administration is not configured")
	}
	return nil
}

func validateReviewTarget(current *appevaluation.ReviewRun, reviewer string, command ReviewCommand) error {
	if current == nil || !current.CanReview {
		return cberrors.WithCode(code.ErrConflict, "AI explanation evaluation is not awaiting review")
	}
	for _, attempt := range current.Attempts {
		if attempt.CaseID != command.CaseID || attempt.Attempt != command.Attempt {
			continue
		}
		for _, review := range attempt.Reviews {
			if review.Role == command.Role {
				return cberrors.WithCode(code.ErrConflict, "AI explanation review role is already recorded for this attempt")
			}
			if strings.TrimSpace(review.Reviewer) == reviewer {
				return cberrors.WithCode(code.ErrConflict, "AI explanation review roles require distinct reviewers")
			}
		}
		return nil
	}
	return cberrors.WithCode(code.ErrInvalidArgument, "AI explanation review target does not exist")
}

func validateEvaluationOrg(current *appevaluation.ReviewRun, actor Actor) error {
	if current == nil {
		return cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation not found")
	}
	if current.RequestedOrgID != 0 && current.RequestedOrgID != actor.OrgID {
		return cberrors.WithCode(code.ErrPermissionDenied, "AI explanation evaluation belongs to another organization")
	}
	return nil
}

func validRole(role domainevaluation.ReviewRole) bool {
	return role == domainevaluation.ReviewRoleAssessmentSemantics || role == domainevaluation.ReviewRoleSafetyProduct
}

func validDecision(decision domainevaluation.ReviewDecision) bool {
	return decision == domainevaluation.ReviewDecisionApprove || decision == domainevaluation.ReviewDecisionReject
}

func mapKnownError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case stderrors.Is(err, domainevaluation.ErrNotFound), stderrors.Is(err, domainprofile.ErrNotFound),
		stderrors.Is(err, domaingeneration.ErrNotFound), stderrors.Is(err, domainrun.ErrNotFound):
		return cberrors.WithCode(code.ErrPageNotFound, "%s", err.Error())
	case stderrors.Is(err, appevaluation.ErrReviewCatalogCursor), stderrors.Is(err, appgovernance.ErrProfileCatalogCursor):
		return cberrors.WithCode(code.ErrInvalidArgument, "%s", err.Error())
	case stderrors.Is(err, domainevaluation.ErrOrgConcurrencyExceeded), stderrors.Is(err, domainevaluation.ErrDailyBudgetExceeded):
		return cberrors.WithCode(code.ErrAIExplanationCapacityExceeded, "%s", err.Error())
	case stderrors.Is(err, domaingeneration.ErrOrgDailyBudgetExceeded), stderrors.Is(err, domaingeneration.ErrUserDailyBudgetExceeded),
		stderrors.Is(err, domaingeneration.ErrAssessmentDailyBudgetExceeded):
		return cberrors.WithCode(code.ErrAIExplanationCapacityExceeded, "%s", err.Error())
	case stderrors.Is(err, domainevaluation.ErrConflict), stderrors.Is(err, domainevaluation.ErrAlreadyExists),
		stderrors.Is(err, domainevaluation.ErrRecoveryNotAllowed), stderrors.Is(err, domainevaluation.ErrCancellationNotAllowed), stderrors.Is(err, domainprofile.ErrConflict),
		stderrors.Is(err, domainprofile.ErrAlreadyExists), stderrors.Is(err, domainprofile.ErrAmbiguousSelector),
		stderrors.Is(err, appgovernance.ErrPublishEvidenceRequired), stderrors.Is(err, appgovernance.ErrReleaseMismatch),
		stderrors.Is(err, appgovernance.ErrSelectorConflict), stderrors.Is(err, appgovernance.ErrProfileFingerprint),
		stderrors.Is(err, domaingeneration.ErrConflict), stderrors.Is(err, domainrun.ErrConflict), stderrors.Is(err, domainrun.ErrRetryNotAllowed):
		return cberrors.WithCode(code.ErrConflict, "%s", err.Error())
	default:
		return err
	}
}

var _ Service = (*service)(nil)
