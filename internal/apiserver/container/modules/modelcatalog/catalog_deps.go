package modelcatalog

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	cachetarget "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// TypologyCacheSignalNotifier 模型目录的缓存信号通知器
type TypologyCacheSignalNotifier interface {
	// NotifyTypologyModelCacheChanged 通知模型目录的缓存发生变化
	NotifyTypologyModelCacheChanged(context.Context, string, string)
}

// CatalogDeps 包含模型目录的依赖
type CatalogDeps struct {
	PublishedLister     port.PublishedModelLister
	PublishedCatalog    port.Catalog
	PublishedWarmer     cachetarget.PublishedModelWarmer
	ModelRepo           port.ModelRepository
	PublishedRepo       port.PublishedModelRepository
	NormRepo            port.NormRepository
	QuestionnaireQuery  questionnaireapp.QuestionnaireQueryService
	CacheSignalNotifier TypologyCacheSignalNotifier
}
