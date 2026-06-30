package personalitymodel

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel/query"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel/shared"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

// PersonalityModelQueryService is the C-side personality model catalog query port.
type PersonalityModelQueryService = query.PersonalityModelQueryService

// NewQueryService creates the personality model query service.
func NewQueryService(lister port.PublishedModelLister) PersonalityModelQueryService {
	return query.NewQueryService(lister)
}

// NewQueryServiceWithAlgorithmLister creates the query service with dynamic category support.
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
