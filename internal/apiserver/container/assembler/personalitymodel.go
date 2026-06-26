package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	appPersonalityModel "github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// PersonalityModelModule hosts C-side personality model catalog services.
type PersonalityModelModule struct {
	QueryService appPersonalityModel.PersonalityModelQueryService
}

// PersonalityModelModuleDeps defines explicit construction dependencies.
type PersonalityModelModuleDeps struct {
	PublishedLister          port.PublishedLister
	PublishedAlgorithmLister port.PublishedAlgorithmLister
}

func NewPersonalityModelModule(deps PersonalityModelModuleDeps) (*PersonalityModelModule, error) {
	if deps.PublishedLister == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "personality model published lister is required")
	}
	var queryService appPersonalityModel.PersonalityModelQueryService
	if deps.PublishedAlgorithmLister != nil {
		queryService = appPersonalityModel.NewQueryServiceWithAlgorithmLister(deps.PublishedLister, deps.PublishedAlgorithmLister)
	} else {
		queryService = appPersonalityModel.NewQueryService(deps.PublishedLister)
	}
	return &PersonalityModelModule{
		QueryService: queryService,
	}, nil
}

func (m *PersonalityModelModule) Cleanup() error { return nil }

func (m *PersonalityModelModule) CheckHealth() error { return nil }

func (m *PersonalityModelModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{Name: "personalitymodel", Version: "1.0.0"}
}
