package assessmentmodel_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestDecisionKindForDefinitionUsesDomainSemantics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		model assessmentmodel.AssessmentModel
		want  binding.DecisionKind
	}{
		{
			name: "scale risk conclusion",
			model: assessmentmodel.AssessmentModel{
				Kind: binding.KindScale,
				DefinitionV2: &definition.Definition{Conclusions: []conclusion.Conclusion{
					conclusion.RiskConclusion{FactorCode: "total"},
				}},
			},
			want: binding.DecisionKindScoreRange,
		},
		{
			name: "behavioral norm",
			model: assessmentmodel.AssessmentModel{
				Kind: binding.KindBehavioralRating,
				DefinitionV2: &definition.Definition{
					Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: "gec", NormTableVersion: "2026"}}},
					Conclusions: []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "gec", Primary: true}},
				},
			},
			want: binding.DecisionKindNormLookup,
		},
		{
			name: "cognitive ability conclusion",
			model: assessmentmodel.AssessmentModel{
				Kind: binding.KindCognitive,
				DefinitionV2: &definition.Definition{Conclusions: []conclusion.Conclusion{
					conclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw},
				}},
			},
			want: binding.DecisionKindAbilityLevel,
		},
		{
			name: "typology type conclusion",
			model: assessmentmodel.AssessmentModel{
				Kind: binding.KindTypology,
				DefinitionV2: &definition.Definition{Conclusions: []conclusion.Conclusion{
					conclusion.TypeConclusion{Decision: conclusion.TypeDecision{Kind: binding.DecisionKindNearestPattern}},
				}},
			},
			want: binding.DecisionKindNearestPattern,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.model.DecisionKindForDefinition()
			if err != nil {
				t.Fatalf("DecisionKindForDefinition: %v", err)
			}
			if got != tc.want {
				t.Fatalf("decision kind = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecisionKindForDefinitionRejectsIncompleteDecision(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		model assessmentmodel.AssessmentModel
	}{
		{
			name:  "scale without risk conclusion",
			model: assessmentmodel.AssessmentModel{Kind: binding.KindScale, DefinitionV2: &definition.Definition{}},
		},
		{
			name:  "cognitive without ability conclusion",
			model: assessmentmodel.AssessmentModel{Kind: binding.KindCognitive, DefinitionV2: &definition.Definition{}},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tc.model.DecisionKindForDefinition(); err == nil {
				t.Fatal("expected incomplete decision to fail")
			}
		})
	}
}

func TestDecisionKindForDefinitionRejectsBehavioralWithoutNormSemantics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		model assessmentmodel.AssessmentModel
	}{
		{
			name: "score range only",
			model: assessmentmodel.AssessmentModel{
				Kind:         binding.KindBehavioralRating,
				DefinitionV2: &definition.Definition{Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total"}}},
			},
		},
		{
			name: "norm refs without primary",
			model: assessmentmodel.AssessmentModel{
				Kind: binding.KindBehavioralRating,
				DefinitionV2: &definition.Definition{
					Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: "gec", NormTableVersion: "2026"}}},
				},
			},
		},
		{
			name: "norm conclusions without refs",
			model: assessmentmodel.AssessmentModel{
				Kind: binding.KindBehavioralRating,
				DefinitionV2: &definition.Definition{
					Conclusions: []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "gec", Primary: true}},
				},
			},
		},
		{
			name: "multiple primary",
			model: assessmentmodel.AssessmentModel{
				Kind: binding.KindBehavioralRating,
				DefinitionV2: &definition.Definition{
					Calibration: definition.Calibration{NormRefs: []norm.Ref{
						{FactorCode: "gec", NormTableVersion: "2026"},
						{FactorCode: "bri", NormTableVersion: "2026"},
					}},
					Conclusions: []conclusion.Conclusion{
						conclusion.NormConclusion{FactorCode: "gec", Primary: true},
						conclusion.NormConclusion{FactorCode: "bri", Primary: true},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tc.model.DecisionKindForDefinition(); err == nil {
				t.Fatal("expected behavioral_rating without complete norm semantics to fail")
			}
		})
	}
}
