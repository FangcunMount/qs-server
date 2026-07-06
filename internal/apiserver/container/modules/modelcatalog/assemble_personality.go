package modelcatalog

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	previewadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/preview"
	appPersonalityModel "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
	appPersonalityCatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality/consumer"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Personality hosts C-side personality model catalog services.
type Personality struct {
	QueryService   appPersonalityCatalog.PersonalityModelQueryService
	CommandService appPersonalityModel.Service
}

// PersonalityDeps defines explicit construction dependencies.
type PersonalityDeps struct {
	PublishedLister          port.PublishedModelLister
	PublishedAlgorithmLister port.PublishedAlgorithmLister
	ModelRepo                port.ModelRepository
	PublishedRepo            port.PublishedModelRepository
	QuestionnaireQuery       questionnaireapp.QuestionnaireQueryService
	CacheSignalNotifier      appPersonalityModel.CacheSignalNotifier
}

// NewPersonality assembles the personality-model catalog capability.
func NewPersonality(deps PersonalityDeps) (*Personality, error) {
	if deps.PublishedLister == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "personality model published lister is required")
	}
	var queryService appPersonalityCatalog.PersonalityModelQueryService
	if deps.PublishedAlgorithmLister != nil {
		queryService = appPersonalityCatalog.NewQueryServiceWithAlgorithmLister(deps.PublishedLister, deps.PublishedAlgorithmLister)
	} else {
		queryService = appPersonalityCatalog.NewQueryService(deps.PublishedLister)
	}
	var commandService appPersonalityModel.Service
	if deps.ModelRepo != nil {
		commandService = appPersonalityModel.NewService(appPersonalityModel.Dependencies{
			ModelRepo:           deps.ModelRepo,
			PublishedRepo:       deps.PublishedRepo,
			QuestionnaireQuery:  deps.QuestionnaireQuery,
			CacheSignalNotifier: deps.CacheSignalNotifier,
			ReportPreviewer:     previewadapter.NewPreviewer(),
		})
	}
	return &Personality{
		QueryService:   queryService,
		CommandService: commandService,
	}, nil
}

// Cleanup releases module resources.
func (m *Personality) Cleanup() error { return nil }

// CheckHealth verifies module health.
func (m *Personality) CheckHealth() error { return nil }

// ModuleInfo returns legacy personality-model module metadata.
func (m *Personality) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{Name: "personalitymodel", Version: "1.0.0"}
}
