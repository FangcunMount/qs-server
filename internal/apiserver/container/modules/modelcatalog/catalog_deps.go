package modelcatalog

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// TypologyCacheSignalNotifier is the best-effort lifecycle side effect for
// the typology catalog projection. It is infrastructure wiring, not a family
// application service.
type TypologyCacheSignalNotifier interface {
	NotifyTypologyModelCacheChanged(context.Context, string, string)
}

// CatalogDeps holds the infrastructure collaborators shared by the five
// actor-oriented assessment-model use cases and their definition strategies.
type CatalogDeps struct {
	PublishedLister     port.PublishedModelLister
	ModelRepo           port.ModelRepository
	PublishedRepo       port.PublishedModelRepository
	NormRepo            port.NormRepository
	QuestionnaireQuery  questionnaireapp.QuestionnaireQueryService
	CacheSignalNotifier TypologyCacheSignalNotifier
}
