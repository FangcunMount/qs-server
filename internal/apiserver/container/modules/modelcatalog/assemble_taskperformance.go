package modelcatalog

import (
	appTaskPerformance "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/taskperformance"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// TaskPerformance hosts task performance model catalog command services.
type TaskPerformance struct {
	CommandService appTaskPerformance.Service
}

// TaskPerformanceDeps defines explicit construction dependencies.
type TaskPerformanceDeps struct {
	ModelRepo     port.ModelRepository
	PublishedRepo port.PublishedModelRepository
	NormRepo      port.NormRepository
}

// NewTaskPerformance assembles the cognitive-model catalog capability.
func NewTaskPerformance(deps TaskPerformanceDeps) (*TaskPerformance, error) {
	var commandService appTaskPerformance.Service
	if deps.ModelRepo != nil {
		commandService = appTaskPerformance.NewService(appTaskPerformance.Dependencies{
			ModelRepo:     deps.ModelRepo,
			PublishedRepo: deps.PublishedRepo,
			NormRepo:      deps.NormRepo,
		})
	}
	return &TaskPerformance{CommandService: commandService}, nil
}

// Cleanup releases module resources.
func (m *TaskPerformance) Cleanup() error { return nil }

// CheckHealth verifies module health.
func (m *TaskPerformance) CheckHealth() error { return nil }
