package configured

import (
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
)

// DetailInput carries the scored state required to assemble a typed detail payload.
type DetailInput struct {
	Payload   *modeltypology.Payload
	Spec      *modeltypology.RuntimeSpec
	Vector    profile.ProfileVector
	Decision  profile.DecisionSpec
	Candidate profile.OutcomeCandidate
	Selected  SelectedOutcome
	Adapter   modeltypology.DetailAdapterKey
}

// SelectedOutcome is the configured-runtime view of a chosen model outcome.
type SelectedOutcome struct {
	Code       string
	Similarity float64
	Trigger    string
	Dimensions []DimensionLevel
}

// DimensionLevel is an intermediate SBTI dimension score used by detail assembly.
type DimensionLevel struct {
	Code     string
	Name     string
	Model    string
	RawScore float64
	Level    string
}

type detailAssemblerFunc func(DetailInput) (any, error)

// DetailAssemblerRegistry resolves detail assemblers by adapter key.
type DetailAssemblerRegistry struct {
	assemblers map[modeltypology.DetailAdapterKey]detailAssemblerFunc
}

// DefaultDetailAssemblerRegistry returns the built-in typology detail assemblers.
func DefaultDetailAssemblerRegistry() DetailAssemblerRegistry {
	return NewDetailAssemblerRegistry()
}

// NewDetailAssemblerRegistry returns the built-in typology detail assemblers.
func NewDetailAssemblerRegistry() DetailAssemblerRegistry {
	return DetailAssemblerRegistry{
		assemblers: map[modeltypology.DetailAdapterKey]detailAssemblerFunc{
			modeltypology.DetailAdapterPersonalityType: assemblePersonalityTypeDetail,
			modeltypology.DetailAdapterTraitProfile:    assembleTraitProfileDetail,
			modeltypology.DetailAdapterMBTI:            assembleMBTIDetail,
			modeltypology.DetailAdapterSBTI:            assembleSBTIDetail,
			modeltypology.DetailAdapterBigFive:         assembleBigFiveDetail,
		},
	}
}

// Len reports how many detail assemblers are registered.
func (r DetailAssemblerRegistry) Len() int {
	return len(r.assemblers)
}

// Register returns a registry copy with an additional or overridden detail assembler.
func (r DetailAssemblerRegistry) Register(key modeltypology.DetailAdapterKey, assembler detailAssemblerFunc) DetailAssemblerRegistry {
	next := DetailAssemblerRegistry{assemblers: make(map[modeltypology.DetailAdapterKey]detailAssemblerFunc, len(r.assemblers)+1)}
	for k, v := range r.assemblers {
		next.assemblers[k] = v
	}
	next.assemblers[key] = assembler
	return next
}

func (r DetailAssemblerRegistry) Assemble(input DetailInput) (any, error) {
	if input.Adapter == "" {
		return nil, fmt.Errorf("detail adapter key is required")
	}
	assembler, ok := r.assemblers[input.Adapter]
	if !ok {
		return nil, fmt.Errorf("unsupported detail adapter key: %s", input.Adapter)
	}
	return assembler(input)
}

func buildSBTIDimensions(input DetailInput) []DimensionLevel {
	if len(input.Selected.Dimensions) > 0 {
		return append([]DimensionLevel(nil), input.Selected.Dimensions...)
	}
	if len(input.Vector.Scores) == 0 {
		return nil
	}
	results := make([]DimensionLevel, 0, len(input.Spec.FactorGraph.DecisionFactorOrder()))
	for _, dimCode := range input.Spec.FactorGraph.DecisionFactorOrder() {
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, dimCode)
		if !ok {
			continue
		}
		score := input.Vector.Scores[profile.FactorID(dimCode)]
		results = append(results, DimensionLevel{
			Code:     dimCode,
			Name:     meta.Name,
			Model:    meta.Model,
			RawScore: score.Raw,
			Level:    profile.LevelForScore(score.Raw, input.Decision.LevelRule),
		})
	}
	return results
}
