package consumer

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer/query"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer/shared"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PersonalityModelQueryService 是C 端 人格模型 目录 查询端口。
type PersonalityModelQueryService = query.PersonalityModelQueryService

// NewQueryService 创建人格模型 查询服务。
func NewQueryService(lister port.PublishedModelLister) PersonalityModelQueryService {
	return query.NewQueryService(lister)
}

// NewQueryServiceWithAlgorithmLister 创建查询服务 使用 动态分类支持。
func NewQueryServiceWithAlgorithmLister(
	lister port.PublishedModelLister,
	algorithmLister port.PublishedAlgorithmLister,
) PersonalityModelQueryService {
	return query.NewQueryServiceWithAlgorithmLister(lister, algorithmLister)
}

type (
	ListPersonalityModelsDTO          = shared.ListPersonalityModelsDTO
	PersonalityModelSummaryResult     = shared.PersonalityModelSummaryResult
	PersonalityModelSummaryListResult = shared.PersonalityModelSummaryListResult
	PersonalityDimensionResult        = shared.PersonalityDimensionResult
	PersonalityOutcomeSummaryResult   = shared.PersonalityOutcomeSummaryResult
	PersonalityModelResult            = shared.PersonalityModelResult
	PersonalityModelCategoryResult    = shared.PersonalityModelCategoryResult
	PersonalityModelCategoriesResult  = shared.PersonalityModelCategoriesResult
)
