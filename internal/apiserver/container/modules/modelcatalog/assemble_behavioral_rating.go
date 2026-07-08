package modelcatalog

import (
	appNorming "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// BehavioralRating hosts behavioral_rating model catalog command services.
type BehavioralRating struct {
	CommandService appNorming.Service
}

// BehavioralRatingDeps defines explicit construction dependencies.
type BehavioralRatingDeps struct {
	ModelRepo     port.ModelRepository
	PublishedRepo port.PublishedModelRepository
}

// NewBehavioralRating assembles the behavioral_rating catalog capability.
func NewBehavioralRating(deps BehavioralRatingDeps) (*BehavioralRating, error) {
	var commandService appNorming.Service
	if deps.ModelRepo != nil {
		commandService = appNorming.NewService(appNorming.Dependencies{
			ModelRepo:     deps.ModelRepo,
			PublishedRepo: deps.PublishedRepo,
		})
	}
	return &BehavioralRating{CommandService: commandService}, nil
}

// Cleanup releases module resources.
func (m *BehavioralRating) Cleanup() error { return nil }

// CheckHealth verifies module health.
func (m *BehavioralRating) CheckHealth() error { return nil }
