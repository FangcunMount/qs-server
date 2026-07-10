package modelcatalog

import (
	previewadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/preview"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
)

// definitionRegistry is the sole composition point for family-specific
// DefinitionV2 strategies. Command services must receive this registry rather
// than constructing family-local registries.
func definitionRegistry(deps Deps) appdefinition.Registry {
	return appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{},
		appdefinition.BehavioralRatingDefinitionHandler{NormRepo: deps.Catalog.NormRepo},
		appdefinition.CognitiveDefinitionHandler{NormRepo: deps.Catalog.NormRepo},
		appdefinition.TypologyDefinitionHandler{QuestionnaireQuery: deps.Catalog.QuestionnaireQuery, ReportPreviewer: previewadapter.NewPreviewer()},
	)
}
