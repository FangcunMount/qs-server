// Package intake contains the Evaluation capability used by answer-sheet orchestration.
package intake

import (
	"context"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type CreateCommand struct {
	OrgID, TesteeID, AnswerSheetID                                               uint64
	QuestionnaireCode, QuestionnaireVersion                                      string
	ModelKind, ModelSubKind, ModelAlgorithm, ModelCode, ModelVersion, ModelTitle *string
	OriginType                                                                   string
	OriginID                                                                     *string
}
type Assessment struct {
	ID, OrgID, TesteeID, AnswerSheetID                          uint64
	QuestionnaireCode, QuestionnaireVersion, OriginType, Status string
	OriginID                                                    *string
	SubmittedAt, EvaluatedAt, FailedAt                          *time.Time
	FailureReason                                               *string
}
type Service interface {
	CreateForAnswerSheet(context.Context, CreateCommand) (*Assessment, error)
	SubmitForEvaluation(context.Context, uint64) (*Assessment, error)
	FindByAnswerSheetID(context.Context, uint64) (*Assessment, error)
}

type assessmentListCache interface {
	Invalidate(context.Context, uint64) error
}
type service struct {
	repo      domainassessment.Repository
	creator   domainassessment.AssessmentCreator
	tx        apptransaction.Runner
	events    EventStager
	cache     assessmentListCache
	immediate *appEventing.ImmediateDispatcher
}
type Option func(*service)

func WithImmediateDispatcher(v *appEventing.ImmediateDispatcher) Option {
	return func(s *service) { s.immediate = v }
}
func NewService(repo domainassessment.Repository, creator domainassessment.AssessmentCreator, tx apptransaction.Runner, events EventStager, cache assessmentListCache, opts ...Option) Service {
	s := &service{repo: repo, creator: creator, tx: tx, events: events, cache: cache}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *service) CreateForAnswerSheet(ctx context.Context, command CreateCommand) (*Assessment, error) {
	if command.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if command.QuestionnaireCode == "" {
		return nil, evalerrors.InvalidArgument("问卷编码不能为空")
	}
	if command.AnswerSheetID == 0 {
		return nil, evalerrors.InvalidArgument("答卷ID不能为空")
	}
	req, err := (assessmentCreateRequestAssembler{}).Assemble(command)
	if err != nil {
		return nil, err
	}
	a, err := s.creator.Create(ctx, req)
	if err != nil {
		return nil, evalerrors.AssessmentCreateFailed(err, "创建测评失败")
	}
	finalizer := assessmentCreateFinalizer{repo: s.repo, txRunner: s.tx, eventStager: s.events, cache: s.cache, immediate: s.immediate}
	if err := finalizer.SaveAndStage(ctx, a, req, command); err != nil {
		return nil, evalerrors.Database(err, "保存测评失败")
	}
	finalizer.InvalidateCache(ctx, command.TesteeID)
	return resultFromDomain(a)
}
func (s *service) SubmitForEvaluation(ctx context.Context, id uint64) (*Assessment, error) {
	a, err := s.repo.FindByID(ctx, meta.FromUint64(id))
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	if err := a.Submit(); err != nil {
		return nil, evalerrors.AssessmentSubmitFailed(err, "提交测评失败")
	}
	finalizer := assessmentSubmitFinalizer{repo: s.repo, txRunner: s.tx, eventStager: s.events, cache: s.cache, immediate: s.immediate}
	if err := finalizer.SaveAndStage(ctx, a); err != nil {
		return nil, evalerrors.Database(err, "保存测评失败")
	}
	finalizer.InvalidateCache(ctx, a.TesteeID().Uint64())
	return resultFromDomain(a)
}
func (s *service) FindByAnswerSheetID(ctx context.Context, id uint64) (*Assessment, error) {
	a, err := s.repo.FindByAnswerSheetID(ctx, domainassessment.NewAnswerSheetRef(meta.FromUint64(id)))
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	return resultFromDomain(a)
}
func resultFromDomain(a *domainassessment.Assessment) (*Assessment, error) {
	if a == nil {
		return nil, nil
	}
	org, err := safeconv.Int64ToUint64(a.OrgID())
	if err != nil {
		return nil, evalerrors.DatabaseMessage("机构ID超出 uint64 范围")
	}
	q := a.QuestionnaireRef()
	return &Assessment{ID: a.ID().Uint64(), OrgID: org, TesteeID: a.TesteeID().Uint64(), QuestionnaireCode: q.Code().String(), QuestionnaireVersion: q.Version(), AnswerSheetID: a.AnswerSheetRef().ID().Uint64(), OriginType: a.OriginType().String(), OriginID: a.OriginID(), Status: a.Status().String(), SubmittedAt: a.SubmittedAt(), EvaluatedAt: a.EvaluatedAt(), FailedAt: a.FailedAt(), FailureReason: a.FailureReason()}, nil
}
