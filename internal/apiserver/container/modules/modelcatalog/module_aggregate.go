package modelcatalog

import (
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appauthoring "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/authoring"
	appmanagement "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/management"
	apppublication "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	appquery "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/query"
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
	Query            assessmentModelApp.CatalogQueryService
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
	management := &appmanagement.Service{
		ModelRepo:       deps.Catalog.ModelRepo,
		Published:       deps.Catalog.PublishedRepo,
		Authorizer:      assessmentModelApp.SnapshotAuthorizer{},
		BindingPolicies: bindings,
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
		ModelRepo:  deps.Catalog.ModelRepo,
		Published:  deps.Catalog.PublishedRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		Registry:   registry,
		Bindings:   bindings,
		Effects:    effects,
	}
	// 查询服务
	query := appquery.NewService(appquery.Dependencies{
		Models:     deps.Catalog.ModelRepo,
		Published:  deps.Catalog.PublishedLister,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		HotRank:    hotRank.ReadModel,
	})
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
		Query:            query,
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
