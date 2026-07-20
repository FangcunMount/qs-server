package modelcatalog

import (
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appauthoring "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/authoring"
	appevolution "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/evolution"
	appmanagement "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/management"
	appnormtable "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/normtable"
	apppublication "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	appquery "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/query"
	apprelease "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/release"
	modelcatalogRuntime "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/runtime"
	cachetarget "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Module 模型目录的组合根
// 包含模型目录的管理、定义、发布和查询用例
type Module struct {
	HotRank          *HotRank
	ModelRepo        modelcatalogport.ModelRepository
	PublishedLister  modelcatalogport.PublishedModelLister
	PublishedCatalog modelcatalogport.Catalog
	PublishedWarmer  cachetarget.PublishedModelWarmer
	Management       assessmentModelApp.CatalogManagementService
	Authoring        *appauthoring.Service
	Publication      assessmentModelApp.PublicationService
	Release          assessmentModelApp.AssessmentReleaseService
	Query            assessmentModelApp.CatalogQueryService
	NormTables       assessmentModelApp.NormTableService
	TitleResolver    assessmentModelApp.PublishedModelTitleResolver
}

// Deps 包含模型目录的基础设施依赖
type Deps struct {
	HotRank   HotRankDeps
	Lifecycle LifecycleDeps
	Catalog   CatalogDeps
}

// New 组合模型目录的应用用例
func New(deps Deps) (*Module, error) {
	// 定义注册表
	registry := definitionRegistry(deps)
	// 问卷绑定策略
	bindings := questionnaireBindingPolicies(deps)
	// 生命周期效果
	effects := lifecycleEffects(deps)
	hotRank := NewHotRank(deps.HotRank)

	// 管理服务
	evolutionPolicy := appevolution.Policy{History: publishedReleaseHistory(deps.Catalog.PublishedLister)}
	management := &appmanagement.Service{
		ModelRepo:       deps.Catalog.ModelRepo,
		Published:       deps.Catalog.PublishedRepo,
		Authorizer:      assessmentModelApp.SnapshotAuthorizer{},
		BindingPolicies: bindings,
		Evolution:       evolutionPolicy,
		Effects:         effects,
	}
	// 定义服务
	authoring := &appauthoring.Service{
		ModelRepo:  deps.Catalog.ModelRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		Registry:   registry,
	}
	// 发布服务
	publication := &apppublication.Service{
		Transactions: deps.Catalog.Transactions,
		ModelRepo:    deps.Catalog.ModelRepo,
		Published:    deps.Catalog.PublishedRepo,
		Authorizer:   assessmentModelApp.SnapshotAuthorizer{},
		Registry:     registry,
		Bindings:     bindings,
		Evolution:    evolutionPolicy,
		Effects:      effects,
	}
	release := apprelease.Service{
		Transactions: deps.Catalog.Transactions,
		Models:       deps.Catalog.ModelRepo, Published: deps.Catalog.PublishedRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{}, Registry: registry,
		Bindings: bindings, Evolution: evolutionPolicy,
		Questionnaires:     deps.Lifecycle.QuestionnairePublisher,
		QuestionnaireQuery: deps.Catalog.QuestionnaireQuery,
		Effects:            effects,
	}
	// 查询服务
	query := appquery.NewService(appquery.Dependencies{
		Models:             deps.Catalog.ModelRepo,
		Published:          deps.Catalog.PublishedLister,
		Authorizer:         assessmentModelApp.SnapshotAuthorizer{},
		HotRank:            hotRank.ReadModel,
		QuestionnaireQuery: deps.Catalog.QuestionnaireQuery,
	})
	normTables := appnormtable.Service{Repository: deps.Catalog.NormRepo, Authorizer: assessmentModelApp.SnapshotAuthorizer{}}
	// 组合模块
	return &Module{
		HotRank:          hotRank,
		ModelRepo:        deps.Catalog.ModelRepo,
		PublishedLister:  deps.Catalog.PublishedLister,
		PublishedCatalog: deps.Catalog.PublishedCatalog,
		PublishedWarmer:  deps.Catalog.PublishedWarmer,
		Management:       management,
		Authoring:        authoring,
		Publication:      publication,
		Release:          release,
		Query:            query,
		NormTables:       normTables,
		TitleResolver:    modelcatalogRuntime.NewTitleResolver(deps.Catalog.PublishedLister),
	}, nil
}

// Cleanup 释放模块资源
func (m *Module) Cleanup() error {
	if m == nil {
		return nil
	}
	if m.HotRank != nil {
		if err := m.HotRank.Cleanup(); err != nil {
			return err
		}
	}
	return nil
}

// CheckHealth 验证模块健康
func (m *Module) CheckHealth() error {
	if m == nil {
		return nil
	}
	if m.HotRank != nil {
		if err := m.HotRank.CheckHealth(); err != nil {
			return err
		}
	}
	return nil
}

// ModuleInfo 返回模块元数据
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "测评解释模型资产模块（量表 + 类型学模型目录）",
	}
}

func publishedReleaseHistory(lister modelcatalogport.PublishedModelLister) modelcatalogport.PublishedReleaseHistoryReader {
	if reader, ok := lister.(modelcatalogport.PublishedReleaseHistoryReader); ok {
		return reader
	}
	return nil
}
