package assessmentmodel

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	appPersonalityModel "github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Personality hosts C-side personality model catalog services.
type Personality struct {
	QueryService appPersonalityModel.PersonalityModelQueryService
}

// PersonalityDeps defines explicit construction dependencies.
type PersonalityDeps struct {
	PublishedLister          port.PublishedLister
	PublishedAlgorithmLister port.PublishedAlgorithmLister
}

// NewPersonality assembles the personality-model catalog capability.
func NewPersonality(deps PersonalityDeps) (*Personality, error) {
	if deps.PublishedLister == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "personality model published lister is required")
	}
	var queryService appPersonalityModel.PersonalityModelQueryService
	if deps.PublishedAlgorithmLister != nil {
		queryService = appPersonalityModel.NewQueryServiceWithAlgorithmLister(deps.PublishedLister, deps.PublishedAlgorithmLister)
	} else {
		queryService = appPersonalityModel.NewQueryService(deps.PublishedLister)
	}
	return &Personality{
		QueryService: queryService,
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
