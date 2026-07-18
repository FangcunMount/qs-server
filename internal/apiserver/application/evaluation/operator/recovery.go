package operator

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/event"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	uuid "github.com/satori/go.uuid"
)

type EventStager interface {
	Stage(context.Context, ...event.DomainEvent) error
}

type RecoveryService interface {
	Retry(context.Context, Actor, uint64) (*Assessment, error)
}

type recoveryService struct {
	assessments domainassessment.Repository
	tx          apptransaction.Runner
	events      EventStager
	authorizer  authorizer
	governed    GovernedRetryService
}

func NewRecoveryService(assessments domainassessment.Repository, tx apptransaction.Runner, events EventStager, access AccessChecker, governed ...GovernedRetryService) RecoveryService {
	service := &recoveryService{assessments: assessments, tx: tx, events: events, authorizer: authorizer{assessments: assessments, access: access}}
	if len(governed) > 0 {
		service.governed = governed[0]
	}
	return service
}

func (s *recoveryService) Retry(ctx context.Context, actor Actor, id uint64) (*Assessment, error) {
	if s.assessments == nil || s.tx == nil || s.events == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment recovery transactional outbox is not configured")
	}
	a, err := s.authorizer.loadAssessment(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if !a.Status().IsFailed() {
		return nil, evalerrors.AssessmentInvalidStatus("只能重试失败的测评")
	}
	if s.governed == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation retry governance is not configured")
	}
	latest, err := s.governed.Latest(ctx, id)
	if err != nil {
		return nil, evalerrors.Database(err, "读取最新测评尝试失败")
	}
	if latest == nil || latest.RetryDecision() == nil || latest.RetryDecision().Disposition != retrygovernance.DispositionManualRequired {
		return nil, evalerrors.AssessmentInvalidStatus("最新失败尝试不需要人工重试")
	}
	requestID := fmt.Sprintf("legacy-evaluation-retry-%s", uuid.Must(uuid.NewV4(), nil).String())
	if _, err := s.governed.Authorize(ctx, actor, GovernedRetryCommand{
		AssessmentID: id, ExpectedAttempt: latest.Attempt().Number, Origin: retrygovernance.AttemptOriginManual,
		RequestID: requestID, Reason: "legacy evaluation retry endpoint",
	}); err != nil {
		return nil, err
	}
	return assessmentFromDomain(a)
}
