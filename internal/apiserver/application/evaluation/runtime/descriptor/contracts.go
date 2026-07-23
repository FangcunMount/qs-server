// Package descriptor owns the application runtime contracts used to execute
// an Evaluation routing decision. Pure identity and routing policy remain in
// domain/evaluation/routing.
package descriptor

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrouting "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type (
	ExecutionIdentity = evalrouting.ExecutionIdentity
	ModelRoute        = evalrouting.ModelRoute
	DescriptorKey     = evalrouting.DescriptorKey
)

var (
	ExecutionIdentityScaleDefault        = evalrouting.ExecutionIdentityScaleDefault
	ExecutionIdentityPersonalityTypology = evalrouting.ExecutionIdentityPersonalityTypology
	ExecutionIdentityCognitiveDefault    = evalrouting.ExecutionIdentityCognitiveDefault
)

func DescriptorKeyFromRoute(route ModelRoute) (DescriptorKey, error) {
	return evalrouting.DescriptorKeyFromRoute(route)
}

func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	return evalrouting.ExecutionFamilyFromRoute(route)
}

type CalculationInput struct {
	Route     ModelRoute
	Execution ExecutionInput
}

type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

type DescriptorExecutor interface {
	Execute(context.Context, RuntimeDescriptor, ExecutionInput) (*domainoutcome.Execution, error)
}

type Calculator interface {
	Calculate(context.Context, CalculationInput) (any, error)
}

type InputAssembler interface {
	Assemble(ExecutionInput) (CalculationInput, error)
}

type OutcomeAssembler interface {
	Assemble(any) (any, error)
}

type OutcomeRequirement string

const (
	OutcomeRequired      OutcomeRequirement = "required"
	OutcomeNotApplicable OutcomeRequirement = "not_applicable"
)

// OutcomeCompletenessPolicy declares the stable result facts a DecisionKind
// must produce before Evaluation can commit its immutable Outcome.
type OutcomeCompletenessPolicy struct {
	PrimaryLevel           OutcomeRequirement
	DimensionLevel         OutcomeRequirement
	ClassificationIdentity OutcomeRequirement
}

// DefaultOutcomeCompletenessPolicy is the one DecisionKind -> committed facts
// contract used by descriptor materialization and commit tests.
func DefaultOutcomeCompletenessPolicy(decision modelcatalog.DecisionKind) OutcomeCompletenessPolicy {
	switch decision {
	case modelcatalog.DecisionKindScoreRange, modelcatalog.DecisionKindNormLookup:
		return OutcomeCompletenessPolicy{
			PrimaryLevel:           OutcomeRequired,
			DimensionLevel:         OutcomeRequired,
			ClassificationIdentity: OutcomeNotApplicable,
		}
	case modelcatalog.DecisionKindAbilityLevel:
		return OutcomeCompletenessPolicy{
			PrimaryLevel:           OutcomeRequired,
			DimensionLevel:         OutcomeNotApplicable,
			ClassificationIdentity: OutcomeNotApplicable,
		}
	default:
		return OutcomeCompletenessPolicy{
			PrimaryLevel:           OutcomeNotApplicable,
			DimensionLevel:         OutcomeNotApplicable,
			ClassificationIdentity: OutcomeRequired,
		}
	}
}

func (p OutcomeCompletenessPolicy) Validate() error {
	for name, requirement := range map[string]OutcomeRequirement{
		"primary_level":           p.PrimaryLevel,
		"dimension_level":         p.DimensionLevel,
		"classification_identity": p.ClassificationIdentity,
	} {
		if requirement != OutcomeRequired && requirement != OutcomeNotApplicable {
			return fmt.Errorf("outcome completeness %s must be required or not_applicable", name)
		}
	}
	return nil
}

// ValidateExecution rejects an incomplete mechanism result before any durable
// Evaluation success fact is written.
func (p OutcomeCompletenessPolicy) ValidateExecution(execution *domainoutcome.Execution) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if execution == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	if p.PrimaryLevel == OutcomeRequired && (execution.Level == nil || execution.Level.Code == "") {
		return fmt.Errorf("evaluation outcome primary level is required")
	}
	if p.DimensionLevel == OutcomeRequired {
		for _, dimension := range execution.Dimensions {
			if dimension.Score == nil && dimension.NormReference == nil {
				continue
			}
			if dimension.Level == nil || dimension.Level.Code == "" {
				return fmt.Errorf("evaluation outcome dimension %q level is required", dimension.Code)
			}
		}
	}
	if p.ClassificationIdentity == OutcomeRequired && !hasClassificationIdentity(execution) {
		return fmt.Errorf("evaluation outcome classification identity is required")
	}
	return nil
}

func hasClassificationIdentity(execution *domainoutcome.Execution) bool {
	if execution.Profile != nil && execution.Profile.Code != "" {
		return true
	}
	if execution.Level != nil && execution.Level.Code != "" {
		return true
	}
	if execution.Summary.PrimaryLabel != "" {
		return true
	}
	for _, dimension := range execution.Dimensions {
		if dimension.Code != "" && (dimension.Kind == domainoutcome.DimensionKindTrait || dimension.Kind == domainoutcome.DimensionKindPole) {
			return true
		}
	}
	return false
}

type RuntimeDescriptor struct {
	Key                DescriptorKey
	AlgorithmFamily    modelcatalog.AlgorithmFamily
	DecisionKind       modelcatalog.DecisionKind
	ExecutionPath      modelcatalog.ExecutionPath
	CompletenessPolicy OutcomeCompletenessPolicy
	InputAssembler     InputAssembler
	Calculator         Calculator
	OutcomeAssembler   OutcomeAssembler
}
