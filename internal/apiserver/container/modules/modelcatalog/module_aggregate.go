package modelcatalog

import (
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appauthoring "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/authoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Module is the assessment-model composition root (scoring + typology/norming/taskperformance catalog).
type Module struct {
	HotRank         *HotRank
	Typology        *Typology
	TaskPerformance *TaskPerformance
	Norming         *Norming
	ModelRepo       modelcatalogport.ModelRepository
	PublishedLister modelcatalogport.PublishedModelLister
	Management      *assessmentModelApp.AssessmentCatalogManagementService
	Authoring       *appauthoring.Service
	Publication     *assessmentModelApp.AssessmentPublicationService
	Query           assessmentModelApp.CatalogQueryService
	TitleResolver   assessmentModelApp.PublishedModelTitleResolver
}

// Deps groups constructor dependencies for both assessment-model capabilities.
type Deps struct {
	HotRank         HotRankDeps
	Lifecycle       LifecycleDeps
	Typology        TypologyDeps
	TaskPerformance TaskPerformanceDeps
	Norming         NormingDeps
}

// New assembles scoring and typology catalog capabilities.
func New(deps Deps) (*Module, error) {
	registry := definitionRegistry(deps)
	bindings := questionnaireBindingPolicies(deps)
	effects := lifecycleEffects(deps)
	hotRank := NewHotRank(deps.HotRank)
	typology, err := NewTypology(deps.Typology)
	if err != nil {
		return nil, err
	}
	taskPerformance, err := NewTaskPerformance(deps.TaskPerformance)
	if err != nil {
		return nil, err
	}
	norming, err := NewNorming(deps.Norming)
	if err != nil {
		return nil, err
	}
	management := &assessmentModelApp.AssessmentCatalogManagementService{
		ModelRepo:       deps.Typology.ModelRepo,
		Published:       deps.Typology.PublishedRepo,
		Authorizer:      assessmentModelApp.SnapshotAuthorizer{},
		BindingPolicies: bindings,
		Effects:         effects,
	}
	authoring := &appauthoring.Service{
		ModelRepo:  deps.Typology.ModelRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		Registry:   registry,
	}
	publication := &assessmentModelApp.AssessmentPublicationService{
		ModelRepo:  deps.Typology.ModelRepo,
		Published:  deps.Typology.PublishedRepo,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		Registry:   registry,
		Bindings:   bindings,
		Effects:    effects,
	}
	query := assessmentModelApp.NewCatalogQueryService(assessmentModelApp.CatalogQueryDependencies{
		Models:     deps.Typology.ModelRepo,
		Published:  deps.Typology.PublishedLister,
		Authorizer: assessmentModelApp.SnapshotAuthorizer{},
		HotRank:    hotRank.ReadModel,
	})
	return &Module{
		HotRank:         hotRank,
		Typology:        typology,
		TaskPerformance: taskPerformance,
		Norming:         norming,
		ModelRepo:       deps.Typology.ModelRepo,
		PublishedLister: deps.Typology.PublishedLister,
		Management:      management,
		Authoring:       authoring,
		Publication:     publication,
		Query:           query,
		TitleResolver:   assessmentModelApp.NewPublishedModelTitleResolver(deps.Typology.PublishedLister),
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
	if m.Typology != nil {
		if err := m.Typology.Cleanup(); err != nil {
			return err
		}
	}
	if m.TaskPerformance != nil {
		if err := m.TaskPerformance.Cleanup(); err != nil {
			return err
		}
	}
	if m.Norming != nil {
		return m.Norming.Cleanup()
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
	if m.Typology != nil {
		if err := m.Typology.CheckHealth(); err != nil {
			return err
		}
	}
	if m.TaskPerformance != nil {
		if err := m.TaskPerformance.CheckHealth(); err != nil {
			return err
		}
	}
	if m.Norming != nil {
		return m.Norming.CheckHealth()
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
