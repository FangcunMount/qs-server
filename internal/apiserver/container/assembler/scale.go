package assembler

import (
	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ScaleModule Scale 模块（量表子域）
// 按照 DDD 限界上下文组织
type ScaleModule struct {
	// service 层 - 按行为者组织
	LifecycleService scaleApp.ScaleLifecycleService
	FactorService    scaleApp.ScaleFactorService
	QueryService     scaleApp.ScaleQueryService
	CategoryService  scaleApp.ScaleCategoryService

	// 事件发布器（由容器统一注入）
	eventPublisher event.EventPublisher
}

// ScaleModuleDeps 定义 Scale 模块的显式构造依赖。
type ScaleModuleDeps struct {
	EventPublisher       event.EventPublisher
	Repo                 scale.Repository
	Reader               scalereadmodel.ScaleReader
	ListCache            scalelistcache.PublishedListCache
	QuestionnaireCatalog questionnairecatalog.Catalog
	RankRedisClient      redis.UniversalClient
	RankCacheBuilder     *keyspace.Builder
	IdentityService      *iam.IdentityService
	HotsetRecorder       cachetarget.HotsetRecorder
}

// NewScaleModule 创建 Scale 模块。
func NewScaleModule(deps ScaleModuleDeps) (*ScaleModule, error) {
	normalized, err := normalizeScaleModuleDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &ScaleModule{}
	module.eventPublisher = normalized.EventPublisher

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	module.LifecycleService = scaleApp.NewLifecycleService(normalized.Repo, normalized.QuestionnaireCatalog, module.eventPublisher, normalized.ListCache)
	module.FactorService = scaleApp.NewFactorService(normalized.Repo, normalized.ListCache, module.eventPublisher)
	hotRankReader := scaleCache.NewRedisScaleHotRankProjection(normalized.RankRedisClient, normalized.RankCacheBuilder)
	module.QueryService = scaleApp.NewQueryService(normalized.Repo, normalized.Reader, normalized.IdentityService, normalized.ListCache, normalized.HotsetRecorder, hotRankReader)
	module.CategoryService = scaleApp.NewCategoryService()

	return module, nil
}

func normalizeScaleModuleDeps(deps ScaleModuleDeps) (ScaleModuleDeps, error) {
	if deps.Repo == nil || deps.Reader == nil {
		return ScaleModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "scale repository and read model are required")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// Cleanup 清理模块资源
func (m *ScaleModule) Cleanup() error {
	return nil
}

// CheckHealth 检查模块健康状态
func (m *ScaleModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *ScaleModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "scale",
		Version:     "2.0.0",
		Description: "量表管理模块（重构版）",
	}
}
