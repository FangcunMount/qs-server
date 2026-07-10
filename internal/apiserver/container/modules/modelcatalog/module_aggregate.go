package modelcatalog

import (
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appauthoring "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/authoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Module is the assessment-model composition root for actor-oriented catalog
// management, definition authoring, publication and query use cases.
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

// Deps groups infrastructure collaborators for the unified assessment catalog.
type Deps struct {
	HotRank   HotRankDeps
	Lifecycle LifecycleDeps
	Catalog   CatalogDeps
}

// New assembles the five assessment-model application use cases.
func New(deps Deps) (*Module, error) {
	registry := definitionRegistry(deps)
	bindings := questionnaireBindingPolicies(deps)
	effects := lifecycleEffects(deps)
	hotRank := NewHotRank(deps.HotRank)
	management := &assessmentModelApp.AssessmentCatalogManagementService{
		ModelRepo:       deps.Catalog.ModelRepo,
		Published:       deps.Catalog.PublishedRepo,
		Authorizer:      assessmentModelApp.SnapshotAuthorizer{},
		BindingPolicies: bindings,
		Effects:         effects,
	}
	authoring := &appauthoring.Service{
		ModelRepo:  deps.Catalog.ModelRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		Registry:   registry,
	}
	publication := &assessmentModelApp.AssessmentPublicationService{
		ModelRepo:  deps.Catalog.ModelRepo,
		Published:  deps.Catalog.PublishedRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		Registry:   registry,
		Bindings:   bindings,
		Effects:    effects,
	}
	query := assessmentModelApp.NewCatalogQueryService(assessmentModelApp.CatalogQueryDependencies{
		Models:     deps.Catalog.ModelRepo,
		Published:  deps.Catalog.PublishedLister,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		HotRank:    hotRank.ReadModel,
	})
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

// Cleanup releases module resources.
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

// CheckHealth verifies module health.
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

// ModuleInfo returns aggregate module metadata.
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "测评解释模型资产模块（量表 + 类型学模型目录）",
	}
}
