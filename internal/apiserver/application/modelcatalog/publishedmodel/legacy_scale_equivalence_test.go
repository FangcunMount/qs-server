package publishedmodel_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestLegacyMedicalScaleAssessmentModelSnapshotEquivalence(t *testing.T) {
	t.Parallel()

	legacyScale := newPublishedLegacyMedicalScaleForEquivalence(t)
	legacyScaleSnapshot := legacyadapter.ScaleSnapshotFromMedicalScale(legacyScale)
	legacyPublished, err := publishedmodel.BuildAssessmentSnapshotFromScale(legacyScaleSnapshot)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshotFromScale: %v", err)
	}

	model, err := legacyadapter.AssessmentModelFromMedicalScale(legacyScale, legacyScale.GetUpdatedAt())
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale: %v", err)
	}
	got, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}

	if got.Version != "1.0.0" {
		t.Fatalf("snapshot version = %q, want legacy scale version", got.Version)
	}
	if !reflect.DeepEqual(got, legacyPublished) {
		t.Fatalf("assessment snapshot mismatch\n got: %#v\nwant: %#v", got, legacyPublished)
	}
}

func newPublishedLegacyMedicalScaleForEquivalence(t *testing.T) *scaledefinition.MedicalScale {
	t.Helper()

	maxScore := 10.0
	totalFactor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("TOTAL"),
		"Total",
		scaledefinition.WithIsTotalScore(true),
		scaledefinition.WithMaxScore(&maxScore),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(0, 10),
				scaledefinition.RiskLevelLow,
				"low",
				"watch",
			),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor(total): %v", err)
	}
	dimensionFactor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("F1"),
		"Factor One",
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1"), meta.NewCode("Q2")}),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategyCnt),
		scaledefinition.WithScoringParams(
			scaledefinition.NewScoringParams().WithCntOptionContents([]string{"yes", "often"}),
		),
		scaledefinition.WithMaxScore(&maxScore),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(0, 5),
				scaledefinition.RiskLevelLow,
				"low",
				"watch",
			),
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(5, 10),
				scaledefinition.RiskLevelHigh,
				"high",
				"act",
			),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor(dimension): %v", err)
	}
	averageFactor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("F2"),
		"Factor Two",
		scaledefinition.WithFactorType(scaledefinition.FactorTypeMultilevel),
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q3"), meta.NewCode("Q4")}),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategyAvg),
		scaledefinition.WithMaxScore(&maxScore),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(0, 5),
				scaledefinition.RiskLevelNone,
				"none",
				"keep",
			),
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(5, 10),
				scaledefinition.RiskLevelMedium,
				"medium",
				"review",
			),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor(average): %v", err)
	}
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCALE_A"),
		"Scale A",
		scaledefinition.WithDescription("legacy scale definition"),
		scaledefinition.WithScaleVersion("1.0.0"),
		scaledefinition.WithQuestionnaire(meta.NewCode("Q1"), "1.0.0"),
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
		scaledefinition.WithCategory(scaledefinition.CategoryADHD),
		scaledefinition.WithStages([]scaledefinition.Stage{scaledefinition.StageDeepAssessment}),
		scaledefinition.WithApplicableAges([]scaledefinition.ApplicableAge{scaledefinition.ApplicableAgeSchoolChild}),
		scaledefinition.WithReporters([]scaledefinition.Reporter{scaledefinition.ReporterParent}),
		scaledefinition.WithTags([]scaledefinition.Tag{scaledefinition.NewTag("screening")}),
		scaledefinition.WithFactors([]*scaledefinition.Factor{totalFactor, dimensionFactor, averageFactor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	return scale
}
