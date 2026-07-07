package configured

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/trait"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// DetailInput carries the scored state required to assemble a typed detail payload.
type DetailInput struct {
	Payload   *modeltypology.Payload
	Spec      *modeltypology.RuntimeSpec
	Vector    trait.ProfileVector
	Decision  trait.DecisionSpec
	Candidate trait.OutcomeCandidate
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
