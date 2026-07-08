package modelcatalog

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	previewadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/preview"
	appTypologyModel "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
	appTypologyCatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Typology hosts C-side typology model catalog services.
type Typology struct {
	QueryService   appTypologyCatalog.TypologyModelQueryService
	CommandService appTypologyModel.Service
}

// TypologyDeps defines explicit construction dependencies.
type TypologyDeps struct {
	PublishedLister          port.PublishedModelLister
	PublishedAlgorithmLister port.PublishedAlgorithmLister
	ModelRepo                port.ModelRepository
	PublishedRepo            port.PublishedModelRepository
	QuestionnaireQuery       questionnaireapp.QuestionnaireQueryService
	CacheSignalNotifier      appTypologyModel.CacheSignalNotifier
}

// NewTypology assembles the personality-model catalog capability.
func NewTypology(deps TypologyDeps) (*Typology, error) {
	if deps.PublishedLister == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "personality model published lister is required")
	}
	var queryService appTypologyCatalog.TypologyModelQueryService
	if deps.PublishedAlgorithmLister != nil {
		queryService = appTypologyCatalog.NewQueryServiceWithAlgorithmLister(deps.PublishedLister, deps.PublishedAlgorithmLister)
	} else {
		queryService = appTypologyCatalog.NewQueryService(deps.PublishedLister)
	}
	var commandService appTypologyModel.Service
	if deps.ModelRepo != nil {
		commandService = appTypologyModel.NewService(appTypologyModel.Dependencies{
			ModelRepo:           deps.ModelRepo,
			PublishedRepo:       deps.PublishedRepo,
			QuestionnaireQuery:  deps.QuestionnaireQuery,
			CacheSignalNotifier: deps.CacheSignalNotifier,
			ReportPreviewer:     previewadapter.NewPreviewer(),
		})
	}
	return &Typology{
		QueryService:   queryService,
		CommandService: commandService,
	}, nil
}

// Cleanup releases module resources.
func (m *Typology) Cleanup() error { return nil }

// CheckHealth verifies module health.
func (m *Typology) CheckHealth() error { return nil }

// ModuleInfo returns legacy personality-model module metadata.
func (m *Typology) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{Name: "typologymodel", Version: "1.0.0"}
}
