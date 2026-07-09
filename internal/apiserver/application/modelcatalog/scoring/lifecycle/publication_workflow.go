package lifecycle

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) publishAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, code)
	if err != nil {
		return nil, err
	}
	if model.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已发布，不能重复发布")
	}
	if err := s.ensureBoundQuestionnairePublished(ctx, code, model); err != nil {
		return nil, err
	}
	if err := assessmentstore.ValidateScaleForPublish(model); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	if err := assessmentstore.SyncScaleMetadataInModel(model); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	if err := assessmentstore.SyncSnapshotStatus(model, scaledefinition.StatusPublished.String()); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}

	if _, err := s.publisher.Publish(ctx, model, publication.PublishOptions{
		ReplaceKind: domain.KindScale,
		AfterPublished: func(ctx context.Context, code, action string) {
			s.notifyCacheChanged(ctx, code, action)
		},
	}); err != nil {
		return nil, s.mapPublicationError(err)
	}

	s.publishScaleChangedEvent(ctx, model, scaledefinition.ChangeActionPublished)
	s.refreshListCache(ctx)
	return assessmentstore.ScaleResult(model)
}

func (s *lifecycleService) unpublishAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, code)
	if err != nil {
		return nil, err
	}
	if model.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能下架")
	}
	if !model.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表未发布，不能下架")
	}

	now := time.Now().UTC()
	if err := model.MarkUnpublished(now); err != nil {
		return nil, shared.WrapAssessmentModelError(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}
	if err := assessmentstore.SyncSnapshotStatus(model, scaledefinition.StatusDraft.String()); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}
	if err := s.publishedRepo.DeletePublished(ctx, domain.KindScale, code); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "清空量表发布快照失败")
	}
	if err := assessmentstore.SaveScale(ctx, s.modelRepo, model); err != nil {
		return nil, err
	}

	s.publishScaleChangedEvent(ctx, model, scaledefinition.ChangeActionUnpublished)
	s.refreshListCache(ctx)
	s.notifyCacheChanged(ctx, code, "unpublish")
	return assessmentstore.ScaleResult(model)
}

func (s *lifecycleService) archiveAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, code)
	if err != nil {
		return nil, err
	}
	if model.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档")
	}

	wasPublished := model.IsPublished()
	now := time.Now().UTC()
	if err := model.MarkArchived(now); err != nil {
		return nil, shared.WrapAssessmentModelError(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}
	if err := assessmentstore.SyncSnapshotStatus(model, scaledefinition.StatusArchived.String()); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}
	if wasPublished {
		if err := s.publishedRepo.DeletePublished(ctx, domain.KindScale, code); err != nil {
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "清空量表发布快照失败")
		}
	}
	if err := assessmentstore.SaveScale(ctx, s.modelRepo, model); err != nil {
		return nil, err
	}

	s.publishScaleChangedEvent(ctx, model, scaledefinition.ChangeActionArchived)
	s.refreshListCache(ctx)
	if wasPublished {
		s.notifyCacheChanged(ctx, code, "archive")
	}
	return assessmentstore.ScaleResult(model)
}

func (s *lifecycleService) mapPublicationError(err error) error {
	var validationErr *appdefinition.ValidationError
	if stderrors.As(err, &validationErr) {
		return shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	if stderrors.Is(err, domain.ErrInvalidArgument) || stderrors.Is(err, domain.ErrInvalidState) {
		return shared.WrapAssessmentModelError(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	return err
}
