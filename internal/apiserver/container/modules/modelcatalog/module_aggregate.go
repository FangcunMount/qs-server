package modelcatalog

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

// Module is the assessment-model composition root (scale + personality catalog).
type Module struct {
	Scale            *Scale
	Personality      *Personality
	Cognitive        *Cognitive
	BehavioralRating *BehavioralRating
}

// Deps groups constructor dependencies for both assessment-model capabilities.
type Deps struct {
	Scale            ScaleDeps
	Personality      PersonalityDeps
	Cognitive        CognitiveDeps
	BehavioralRating BehavioralRatingDeps
}

// New assembles scale and personality catalog capabilities.
func New(deps Deps) (*Module, error) {
	scale, err := NewScale(deps.Scale)
	if err != nil {
		return nil, err
	}
	personality, err := NewPersonality(deps.Personality)
	if err != nil {
		return nil, err
	}
	cognitive, err := NewCognitive(deps.Cognitive)
	if err != nil {
		return nil, err
	}
	behavioralRating, err := NewBehavioralRating(deps.BehavioralRating)
	if err != nil {
		return nil, err
	}
	return &Module{
		Scale:            scale,
		Personality:      personality,
		Cognitive:        cognitive,
		BehavioralRating: behavioralRating,
	}, nil
}

// Cleanup releases module resources.
func (m *Module) Cleanup() error {
	if m == nil {
		return nil
	}
	if m.Scale != nil {
		if err := m.Scale.Cleanup(); err != nil {
			return err
		}
	}
	if m.Personality != nil {
		if err := m.Personality.Cleanup(); err != nil {
			return err
		}
	}
	if m.Cognitive != nil {
		if err := m.Cognitive.Cleanup(); err != nil {
			return err
		}
	}
	if m.BehavioralRating != nil {
		return m.BehavioralRating.Cleanup()
	}
	return nil
}

// CheckHealth verifies module health.
func (m *Module) CheckHealth() error {
	if m == nil {
		return nil
	}
	if m.Scale != nil {
		if err := m.Scale.CheckHealth(); err != nil {
			return err
		}
	}
	if m.Personality != nil {
		if err := m.Personality.CheckHealth(); err != nil {
			return err
		}
	}
	if m.Cognitive != nil {
		if err := m.Cognitive.CheckHealth(); err != nil {
			return err
		}
	}
	if m.BehavioralRating != nil {
		return m.BehavioralRating.CheckHealth()
	}
	return nil
}

// ModuleInfo returns aggregate module metadata.
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "测评解释模型资产模块（量表 + 人格模型目录）",
	}
}
