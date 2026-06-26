package personalitymodel

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel/query"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel/shared"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

// PersonalityModelQueryService is the C-side personality model catalog query port.
type PersonalityModelQueryService = query.PersonalityModelQueryService

// NewQueryService creates the personality model query service.
func NewQueryService(lister port.PublishedLister) PersonalityModelQueryService {
	return query.NewQueryService(lister)
}

type (
	ListPersonalityModelsDTO            = shared.ListPersonalityModelsDTO
	PersonalityModelSummaryResult       = shared.PersonalityModelSummaryResult
	PersonalityModelSummaryListResult   = shared.PersonalityModelSummaryListResult
	PersonalityDimensionResult          = shared.PersonalityDimensionResult
	PersonalityOutcomeSummaryResult     = shared.PersonalityOutcomeSummaryResult
	PersonalityModelResult              = shared.PersonalityModelResult
	PersonalityModelCategoryResult      = shared.PersonalityModelCategoryResult
	PersonalityModelCategoriesResult    = shared.PersonalityModelCategoriesResult
)
