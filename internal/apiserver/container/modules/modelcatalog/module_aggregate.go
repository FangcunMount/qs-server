package modelcatalog

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

// Module is the assessment-model composition root (scoring + typology/norming/taskperformance catalog).
type Module struct {
	Scoring         *Scoring
	Typology        *Typology
	TaskPerformance *TaskPerformance
	Norming         *Norming
}

// Deps groups constructor dependencies for both assessment-model capabilities.
type Deps struct {
	Scoring         ScoringDeps
	Typology        TypologyDeps
	TaskPerformance TaskPerformanceDeps
	Norming         NormingDeps
}

// New assembles scoring and typology catalog capabilities.
func New(deps Deps) (*Module, error) {
	scoring, err := NewScoring(deps.Scoring)
	if err != nil {
		return nil, err
	}
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
	return &Module{
		Scoring:         scoring,
		Typology:        typology,
		TaskPerformance: taskPerformance,
		Norming:         norming,
	}, nil
}

// Cleanup releases module resources.
func (m *Module) Cleanup() error {
	if m == nil {
		return nil
	}
	if m.Scoring != nil {
		if err := m.Scoring.Cleanup(); err != nil {
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
	if m.Scoring != nil {
		if err := m.Scoring.CheckHealth(); err != nil {
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
