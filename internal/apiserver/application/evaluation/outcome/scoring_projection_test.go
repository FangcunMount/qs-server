package outcome

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBehavioralRatingModelIdentitySurvivesExecutionProjectionRoundTrip(t *testing.T) {
	t.Parallel()

	for _, algorithm := range []modelcatalog.Algorithm{
		modelcatalog.AlgorithmBrief2,
		modelcatalog.AlgorithmSPMSensory,
	} {
		algorithm := algorithm
		t.Run(algorithm.String(), func(t *testing.T) {
			t.Parallel()

			modelRef := assessment.NewEvaluationModelRefWithIdentity(
				modelcatalog.KindBehavioralRating,
				modelcatalog.SubKindEmpty,
				algorithm,
				meta.ZeroID,
				meta.NewCode("behavioral-model"),
				"v1",
				"Behavioral Model",
			)
			if routeAlgorithm := modelRef.ExecutionIdentity().Algorithm; routeAlgorithm != algorithm {
				t.Fatalf("route algorithm = %s, want %s", routeAlgorithm, algorithm)
			}

			executionRef := ModelRefFromAssessment(modelRef)
			if executionRef.Algorithm() != algorithm {
				t.Fatalf("execution algorithm = %s, want %s", executionRef.Algorithm(), algorithm)
			}

			execution := domainoutcome.NewExecution(
				executionRef,
				domainoutcome.Summary{PrimaryLabel: "normal"},
				domainoutcome.Detail{Kind: modelcatalog.KindBehavioralRating},
			)
			projection := ScoringProjectionFromExecution(execution)
			if projection.ModelRef.Algorithm() != algorithm {
				t.Fatalf("projection algorithm = %s, want %s", projection.ModelRef.Algorithm(), algorithm)
			}
		})
	}
}

func TestModelRefFromAssessmentKeepsEmptyAlgorithm(t *testing.T) {
	t.Parallel()

	modelRef := assessment.NewScaleEvaluationModelRef(
		meta.ZeroID,
		meta.NewCode("SDS"),
		"1.0.0",
		"SDS",
	)

	executionRef := ModelRefFromAssessment(modelRef)
	if executionRef.Algorithm() != "" {
		t.Fatalf("execution algorithm = %q, want empty (no invent)", executionRef.Algorithm())
	}
	if executionRef.Kind() != modelcatalog.KindScale {
		t.Fatalf("kind = %s, want scale", executionRef.Kind())
	}
}
