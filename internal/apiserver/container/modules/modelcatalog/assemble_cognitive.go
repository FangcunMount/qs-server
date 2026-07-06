package modelcatalog

import (
	appCognitive "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/cognitive"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Cognitive hosts cognitive model catalog command services.
type Cognitive struct {
	CommandService appCognitive.Service
}

// CognitiveDeps defines explicit construction dependencies.
type CognitiveDeps struct {
	ModelRepo     port.ModelRepository
	PublishedRepo port.PublishedModelRepository
}

// NewCognitive assembles the cognitive-model catalog capability.
func NewCognitive(deps CognitiveDeps) (*Cognitive, error) {
	var commandService appCognitive.Service
	if deps.ModelRepo != nil {
		commandService = appCognitive.NewService(appCognitive.Dependencies{
			ModelRepo:     deps.ModelRepo,
			PublishedRepo: deps.PublishedRepo,
		})
	}
	return &Cognitive{CommandService: commandService}, nil
}

// Cleanup releases module resources.
func (m *Cognitive) Cleanup() error { return nil }

// CheckHealth verifies module health.
func (m *Cognitive) CheckHealth() error { return nil }
