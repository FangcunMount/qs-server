package administration

import (
	"context"
	"strings"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type EvaluationV2Canceller interface {
	Cancel(context.Context, meta.ID, int64, string, string, bool, time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error)
}
type EvaluationV2ListQuery struct {
	Status *domainevaluation.EvidenceStatus
	Cursor string
	Limit  int
}
type CancelEvaluationV2Command struct {
	ExpectedVersion int64
	Reason          string
	Discard         bool
}

func WithEvaluationV2Management(catalog appevaluation.EvidenceV2Catalog, cancel EvaluationV2Canceller) Option {
	return func(s *service) { s.catalogV2 = catalog; s.cancelV2 = cancel }
}
func (s *service) ListEvaluationsV2(ctx context.Context, actor Actor, query EvaluationV2ListQuery) (*appevaluation.EvidenceV2Page, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || query.Limit < 0 || query.Limit > 100 || (query.Status != nil && !query.Status.IsValid()) {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "invalid v2 Run list query")
	}
	if s == nil || s.catalogV2 == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "v2 Run catalog is disabled")
	}
	if err := s.access.AuthorizeRead(ctx, actor); err != nil {
		return nil, err
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	items, next, err := s.catalogV2.ListEvidenceV2(ctx, actor.OrgID, query.Status, query.Cursor, query.Limit)
	if err != nil {
		return nil, mapKnownError(err)
	}
	return &appevaluation.EvidenceV2Page{Items: items, NextCursor: next}, nil
}
func (s *service) CancelEvaluationV2(ctx context.Context, actor Actor, runID meta.ID, command CancelEvaluationV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	command.Reason = strings.TrimSpace(command.Reason)
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 || runID.IsZero() || command.ExpectedVersion < 1 || command.Reason == "" || len(command.Reason) > maxAuditReasonLength {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "invalid v2 Run cancellation")
	}
	if s == nil || s.cancelV2 == nil || s.evidenceV2 == nil || s.access == nil || s.now == nil {
		return nil, cberrors.WithCode(code.ErrUnsupportedOperation, "v2 Run cancellation is disabled")
	}
	if err := s.access.AuthorizeGovernance(ctx, actor); err != nil {
		return nil, err
	}
	current, err := s.evidenceV2.Find(ctx, runID)
	if err != nil {
		return nil, mapKnownError(err)
	}
	if err := validateEvaluationV2Org(current, actor); err != nil {
		return nil, err
	}
	value, err := s.cancelV2.Cancel(ctx, runID, command.ExpectedVersion, actor.Subject(), command.Reason, command.Discard, s.now().UTC())
	return value, mapKnownError(err)
}
