package modelcatalog

import (
	appNorming "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Norming hosts norming model catalog command services.
type Norming struct {
	CommandService appNorming.Service
}

// NormingDeps defines explicit construction dependencies.
type NormingDeps struct {
	ModelRepo     port.ModelRepository
	PublishedRepo port.PublishedModelRepository
	NormRepo      port.NormRepository
}

// NewNorming assembles the behavioral_rating catalog capability.
func NewNorming(deps NormingDeps) (*Norming, error) {
	var commandService appNorming.Service
	if deps.ModelRepo != nil {
		commandService = appNorming.NewService(appNorming.Dependencies{
			ModelRepo:     deps.ModelRepo,
			PublishedRepo: deps.PublishedRepo,
			NormRepo:      deps.NormRepo,
		})
	}
	return &Norming{CommandService: commandService}, nil
}

// Cleanup releases module resources.
func (m *Norming) Cleanup() error { return nil }

// CheckHealth verifies module health.
func (m *Norming) CheckHealth() error { return nil }
