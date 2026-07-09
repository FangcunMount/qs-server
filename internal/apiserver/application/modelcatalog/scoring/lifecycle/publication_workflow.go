package lifecycle

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) usesAssessmentModelPublishStore() bool {
	return s != nil && s.modelRepo != nil && s.publishedRepo != nil && s.publisher.ModelRepo != nil && s.publisher.Repo != nil
}

func (s *lifecycleService) publishAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, code)
	if err != nil {
		return nil, err
	}
	if model.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已发布，不能重复发布")
	}

	scale, err := legacyadapter.MedicalScaleFromAssessmentModel(model)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	if err := s.ensureBoundQuestionnairePublished(ctx, code, scale); err != nil {
		return nil, err
	}
	if err := legacyadapter.SyncAssessmentModelFromMedicalScale(model, scale, time.Now().UTC()); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	if err := s.lifecycle.Publish(ctx, scale); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	if err := legacyadapter.SyncAssessmentModelFromMedicalScale(model, scale, time.Now().UTC()); err != nil {
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

	s.publishEvents(ctx, scale)
	s.refreshListCache(ctx)
	return assessmentstore.ScaleResult(model)
}

func (s *lifecycleService) unpublishAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, code)
	if err != nil {
		return nil, err
	}

	scale, err := legacyadapter.MedicalScaleFromAssessmentModel(model)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}
	if err := s.lifecycle.Unpublish(ctx, scale); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}

	now := time.Now().UTC()
	if err := legacyadapter.SyncAssessmentModelFromMedicalScale(model, scale, now); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}
	if err := s.publishedRepo.DeletePublished(ctx, domain.KindScale, code); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "清空量表发布快照失败")
	}
	if err := assessmentstore.SaveScale(ctx, s.modelRepo, model); err != nil {
		return nil, err
	}

	s.publishEvents(ctx, scale)
	s.refreshListCache(ctx)
	s.notifyCacheChanged(ctx, code, "unpublish")
	return assessmentstore.ScaleResult(model)
}

func (s *lifecycleService) archiveAssessmentModel(ctx context.Context, code string) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, code)
	if err != nil {
		return nil, err
	}

	wasPublished := model.IsPublished()
	scale, err := legacyadapter.MedicalScaleFromAssessmentModel(model)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}
	if err := s.lifecycle.Archive(ctx, scale); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}

	now := time.Now().UTC()
	if err := legacyadapter.SyncAssessmentModelFromMedicalScale(model, scale, now); err != nil {
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

	s.publishEvents(ctx, scale)
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
