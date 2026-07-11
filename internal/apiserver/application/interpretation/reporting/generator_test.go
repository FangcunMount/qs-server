package reporting

import (
	"context"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type generatorScaleBuilder struct{ draft *report.Draft }

func (b generatorScaleBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}
func (b generatorScaleBuilder) Key() evaluation.ExecutionIdentity { return b.ExecutionIdentity() }
func (generatorScaleBuilder) ReportType() domainreport.ReportType {
	return domainreport.ReportTypeStandard
}
func (generatorScaleBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainreport.ReportTypeStandard,
	}
}
func (b generatorScaleBuilder) Build(context.Context, interpinput.InterpretationInput) (*report.Draft, error) {
	return b.draft, nil
}

func TestGeneratorEmitsInterpretationReportEvents(t *testing.T) {
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(9)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7)),
		assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(0, meta.NewCode("S-1"), "1.0.0", "Scale")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	execution := domainoutcome.NewExecution(
		evaloutcome.ModelRefFromAssessment(*a.EvaluationModelRef()),
		domainoutcome.Summary{PrimaryLabel: "ok"}, domainoutcome.Detail{Kind: modelcatalog.KindScale},
	)
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 7}
	if err := a.ApplyScoringProjection(evaloutcome.ScoringProjectionFromExecution(execution)); err != nil {
		t.Fatal(err)
	}
	outcome := evaloutcome.Outcome{
		Assessment: a,
		Execution:  execution,
		RuntimeDescriptorKey: evalpipeline.RuntimeDescriptorKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
			DecisionKind:    modelcatalog.DecisionKindScoreRange,
		},
	}
	builders, err := NewReportBuilderRegistry(generatorScaleBuilder{draft: report.NewDraft(report.Content{Model: report.ModelIdentity{Title: "Scale", Code: "S-1"}, PrimaryScore: report.NewRawTotalScore(7, nil), Level: domainreport.LevelFromRisk(domainreport.RiskLevelLow), Conclusion: "ok"})})
	if err != nil {
		t.Fatal(err)
	}
	generator, err := NewGenerator(builders)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := generator.Generate(context.Background(), outcome)
	if err != nil {
		t.Fatal(err)
	}
	if len(generation.Events) != 2 {
		t.Fatalf("report events = %d, want interpretation.report.generated + footprint", len(generation.Events))
	}
}
