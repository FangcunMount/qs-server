package modelcatalog

import (
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	previewadapter "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog/preview"
)

// definitionRegistry 模型目录的定义注册表
// 是模型目录的唯一组合点，用于组合模型目录的定义
// 命令服务必须接收这个注册表，而不是构造家族本地注册表
func definitionRegistry(deps Deps) appdefinition.Registry {
	return appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{},
		appdefinition.BehavioralRatingDefinitionHandler{NormRepo: deps.Catalog.NormRepo},
		appdefinition.CognitiveDefinitionHandler{NormRepo: deps.Catalog.NormRepo},
		appdefinition.TypologyDefinitionHandler{QuestionnaireQuery: deps.Catalog.QuestionnaireQuery, ReportPreviewer: previewadapter.NewPreviewer()},
	)
}
