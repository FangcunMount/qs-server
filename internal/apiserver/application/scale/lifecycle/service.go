package lifecycle

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	domscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 量表生命周期服务实现
// 行为者：量表设计者/管理员
type lifecycleService struct {
	repo                 lifecycleRepository
	questionnaireCatalog questionnairecatalog.Catalog
	lifecycle            domscale.Lifecycle
	baseInfo             domscale.BaseInfo
	eventPublisher       event.EventPublisher
	listCache            scalelistcache.PublishedListCache
}

type lifecycleRepository interface {
	Create(ctx context.Context, scale *domscale.MedicalScale) error
	FindByCode(ctx context.Context, code string) (*domscale.MedicalScale, error)
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*domscale.MedicalScale, error)
	Update(ctx context.Context, scale *domscale.MedicalScale) error
	Remove(ctx context.Context, code string) error
}

// NewService 创建量表生命周期应用服务。
func NewService(
	repo lifecycleRepository,
	questionnaireCatalog questionnairecatalog.Catalog,
	eventPublisher event.EventPublisher,
	listCache scalelistcache.PublishedListCache,
) ports.ScaleLifecycleService {
	return &lifecycleService{
		repo:                 repo,
		questionnaireCatalog: questionnaireCatalog,
		lifecycle:            domscale.NewLifecycle(),
		baseInfo:             domscale.BaseInfo{},
		eventPublisher:       eventPublisher,
		listCache:            listCache,
	}
}

// Publish 发布量表
func (s *lifecycleService) Publish(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	if err := s.resolveQuestionnaireBinding().ensureQuestionnaireVersion(ctx, code, m); err != nil {
		return nil, err
	}

	return s.executeLifecycleOperation(ctx, m, func(ctx context.Context, med *domscale.MedicalScale) error {
		return s.lifecycle.Publish(ctx, med)
	})
}

// Unpublish 下架量表
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	return s.executeLifecycleOperation(ctx, m, func(ctx context.Context, med *domscale.MedicalScale) error {
		return s.lifecycle.Unpublish(ctx, med)
	})
}

// Archive 归档量表
func (s *lifecycleService) Archive(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	return s.executeLifecycleOperation(ctx, m, func(ctx context.Context, med *domscale.MedicalScale) error {
		return s.lifecycle.Archive(ctx, med)
	})
}

func (s *lifecycleService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	shared.LogScaleListCacheError(ctx, s.listCache.Rebuild(ctx))
}

func (s *lifecycleService) generateScaleCode(code string) (meta.Code, error) {
	if code != "" {
		return meta.NewCode(code), nil
	}
	return meta.GenerateCode()
}

func (s *lifecycleService) getScaleByCode(ctx context.Context, code string) (*domscale.MedicalScale, error) {
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	return m, nil
}

type lifecycleOperation func(ctx context.Context, med *domscale.MedicalScale) error

func (s *lifecycleService) executeLifecycleOperation(
	ctx context.Context,
	m *domscale.MedicalScale,
	operation lifecycleOperation,
) (*shared.ScaleResult, error) {
	if err := operation(ctx, m); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}

	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表状态失败")
	}

	s.publishEvents(ctx, m)
	s.refreshListCache(ctx)

	return shared.ToScaleResult(m), nil
}

func (s *lifecycleService) publishEvents(ctx context.Context, m *domscale.MedicalScale) {
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, m, nil, nil)
}
