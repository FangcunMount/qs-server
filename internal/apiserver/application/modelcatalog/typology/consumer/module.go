package consumer

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer/query"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer/shared"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// TypologyModelQueryService 是 C 端 typology 模型目录查询端口。
type TypologyModelQueryService = query.TypologyModelQueryService

// NewQueryService 创建 typology 模型查询服务。
func NewQueryService(lister port.PublishedModelLister) TypologyModelQueryService {
	return query.NewQueryService(lister)
}

// NewQueryServiceWithAlgorithmLister 创建查询服务，支持动态分类。
func NewQueryServiceWithAlgorithmLister(
	lister port.PublishedModelLister,
	algorithmLister port.PublishedAlgorithmLister,
) TypologyModelQueryService {
	return query.NewQueryServiceWithAlgorithmLister(lister, algorithmLister)
}

type (
	ListTypologyModelsDTO          = shared.ListTypologyModelsDTO
	TypologyModelSummaryResult     = shared.TypologyModelSummaryResult
	TypologyModelSummaryListResult = shared.TypologyModelSummaryListResult
	TypologyDimensionResult        = shared.TypologyDimensionResult
	TypologyOutcomeSummaryResult   = shared.TypologyOutcomeSummaryResult
	TypologyModelResult            = shared.TypologyModelResult
	TypologyModelCategoryResult    = shared.TypologyModelCategoryResult
	TypologyModelCategoriesResult  = shared.TypologyModelCategoriesResult
)
