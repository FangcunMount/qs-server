package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ScoringProjectionFromExecution narrows the canonical Execution to the only
// fields the Assessment aggregate needs for its evaluated transition.
func ScoringProjectionFromExecution(execution *domainoutcome.Execution) assessment.ScoringProjection {
	if execution == nil {
		return assessment.ScoringProjection{}
	}
	projection := assessment.ScoringProjection{
		ModelRef: AssessmentModelRefFromExecution(execution.ModelRef),
		Summary: assessment.ResultSummary{
			PrimaryLabel: execution.Summary.PrimaryLabel,
			Score:        cloneFloat(execution.Summary.Score),
			Level:        cloneString(execution.Summary.Level),
			Tags:         append([]string(nil), execution.Summary.Tags...),
		},
	}
	if execution.Primary != nil {
		projection.Score = cloneFloat(&execution.Primary.Value)
	} else {
		projection.Score = cloneFloat(execution.Summary.Score)
	}
	if execution.Level != nil {
		projection.Level = execution.Level.Code
	} else if execution.Summary.Level != nil {
		projection.Level = *execution.Summary.Level
	}
	return projection
}

// AssessmentModelRefFromExecution maps immutable model identity into the
// Assessment-owned identity value without copying execution details.
func AssessmentModelRefFromExecution(ref domainoutcome.ModelRef) assessment.EvaluationModelRef {
	return assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKind(ref.Kind()), ref.SubKind(), ref.Algorithm(),
		meta.ZeroID, ref.Code(), ref.Version(), ref.Title(),
	)
}

// ModelRefFromAssessment captures Assessment model identity for a new
// canonical Execution.
func ModelRefFromAssessment(ref assessment.EvaluationModelRef) domainoutcome.ModelRef {
	kind, subKind, algorithm := modelcatalog.Kind(ref.Kind()), ref.SubKind(), ref.Algorithm()
	return domainoutcome.ModelRef{
		ModelKind: kind, ModelSubKind: subKind, ModelAlgorithm: algorithm,
		ModelCode: ref.Code().String(), ModelVersion: ref.Version(), ModelTitle: ref.Title(),
	}
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
