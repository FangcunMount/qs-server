package modelcatalog

import (
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appauthoring "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/authoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Module 模型目录的组合根
// 包含模型目录的管理、定义、发布和查询用例
type Module struct {
	HotRank         *HotRank
	ModelRepo       modelcatalogport.ModelRepository
	PublishedLister modelcatalogport.PublishedModelLister
	Management      *assessmentModelApp.AssessmentCatalogManagementService
	Authoring       *appauthoring.Service
	Publication     *assessmentModelApp.AssessmentPublicationService
	Query           assessmentModelApp.CatalogQueryService
	TitleResolver   assessmentModelApp.PublishedModelTitleResolver
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
	management := &assessmentModelApp.AssessmentCatalogManagementService{
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
	publication := &assessmentModelApp.AssessmentPublicationService{
		ModelRepo:  deps.Catalog.ModelRepo,
		Published:  deps.Catalog.PublishedRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		Registry:   registry,
		Bindings:   bindings,
		Effects:    effects,
	}
	// 查询服务
	query := assessmentModelApp.NewCatalogQueryService(assessmentModelApp.CatalogQueryDependencies{
		Models:     deps.Catalog.ModelRepo,
		Published:  deps.Catalog.PublishedLister,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		HotRank:    hotRank.ReadModel,
	})
	// 组合模块
	return &Module{
		HotRank:         hotRank,
		ModelRepo:       deps.Catalog.ModelRepo,
		PublishedLister: deps.Catalog.PublishedLister,
		Management:      management,
		Authoring:       authoring,
		Publication:     publication,
		Query:           query,
		TitleResolver:   assessmentModelApp.NewPublishedModelTitleResolver(deps.Catalog.PublishedLister),
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
