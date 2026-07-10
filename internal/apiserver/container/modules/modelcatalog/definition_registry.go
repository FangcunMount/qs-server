package modelcatalog

import (
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	appNorming "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	appTaskPerformance "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/taskperformance"
	appTypology "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
)

// definitionRegistry is the sole composition point for family-specific
// DefinitionV2 strategies. Command services must receive this registry rather
// than constructing family-local registries.
func definitionRegistry(deps Deps) appdefinition.Registry {
	return appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{},
		appNorming.DefinitionHandler{NormRepo: deps.Norming.NormRepo},
		appTaskPerformance.DefinitionHandler{NormRepo: deps.TaskPerformance.NormRepo},
		appTypology.DefinitionHandler{QuestionnaireQuery: deps.Typology.QuestionnaireQuery},
	)
}
