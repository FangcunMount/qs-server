package operator

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpolicy"
	"github.com/FangcunMount/qs-server/pkg/event"
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
}

func NewRecoveryService(assessments domainassessment.Repository, tx apptransaction.Runner, events EventStager, access AccessChecker) RecoveryService {
	return &recoveryService{assessments: assessments, tx: tx, events: events, authorizer: authorizer{assessments: assessments, access: access}}
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
	if err := a.RetryFromFailed(); err != nil {
		return nil, evalerrors.WrapAssessmentInvalidStatus(err, "重置测评状态失败")
	}
	err = s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.assessments.Save(txCtx, a); err != nil {
			return err
		}
		events := outboxpolicy.Filter(a.Events())
		if len(events) == 0 {
			return nil
		}
		return s.events.Stage(txCtx, events...)
	})
	if err != nil {
		return nil, evalerrors.Database(err, "保存测评失败")
	}
	a.ClearEvents()
	return assessmentFromDomain(a)
}
