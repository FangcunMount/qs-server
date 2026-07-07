// Package preview implements the model-catalog report-preview port on the
// evaluation side: it builds a synthetic submitted assessment, runs the typology
// executor, builds the interpretation report and projects the outcome. This
// keeps the model-catalog free of evaluation/interpretation implementation
// details while the evaluation module owns "how a report is produced".
package preview

import (
	"context"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Previewer implements modelpreview.ReportPreviewer.
type Previewer struct{}

// NewPreviewer builds a report previewer.
func NewPreviewer() *Previewer { return &Previewer{} }

var _ modelpreview.ReportPreviewer = (*Previewer)(nil)

// PreviewReport runs the typology executor and report builder against synthetic
// assessment facts and projects a preview result.
func (p *Previewer) PreviewReport(ctx context.Context, req modelpreview.Request) (*modelpreview.Result, error) {
	submitted, err := buildSubmittedAssessment(req)
	if err != nil {
		return nil, err
	}
	executor, err := evalregistry.NewConfiguredTypologyExecutor()
	if err != nil {
		return nil, err
	}
	outcome, err := executor.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: submitted,
		Input:      req.Input,
	})
	if err != nil {
		return nil, err
	}
	reportBuilder, err := evalregistry.NewConfiguredReportBuilder()
	if err != nil {
		return nil, err
	}
	report, err := reportBuilder.Build(ctx, evaloutcome.Outcome{
		Assessment: submitted,
		Input:      req.Input,
		Execution:  outcome,
	})
	if err != nil {
		return nil, err
	}
	result := &modelpreview.Result{
		Scores: scoresFromOutcome(outcome),
		Report: report,
	}
	result.OutcomeCode, result.OutcomeTitle = outcomeIdentity(outcome)
	if len(result.Scores) == 0 {
		result.Scores = nil
	}
	return result, nil
}

func buildSubmittedAssessment(req modelpreview.Request) (*assessment.Assessment, error) {
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		req.SubKind,
		req.Algorithm,
		meta.ID(0),
		meta.NewCode(req.Code),
		req.Version,
		req.Title,
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode(req.QuestionnaireCode), req.QuestionnaireVersion),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(1)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		return nil, err
	}
	if err := a.Submit(); err != nil {
		return nil, err
	}
	a.ClearEvents()
	return a, nil
}

func outcomeIdentity(outcome *assessment.AssessmentOutcome) (string, string) {
	if outcome == nil {
		return "", ""
	}
	if outcome.Profile != nil {
		return outcome.Profile.Code, outcome.Profile.Name
	}
	if outcome.Level != nil {
		return outcome.Level.Code, outcome.Level.Label
	}
	return "", ""
}

func scoresFromOutcome(outcome *assessment.AssessmentOutcome) map[string]float64 {
	if outcome == nil {
		return nil
	}
	scores := map[string]float64{}
	if outcome.Primary != nil {
		scores["primary"] = outcome.Primary.Value
	}
	for _, dim := range outcome.Dimensions {
		if dim.Score != nil && dim.Code != "" {
			scores[dim.Code] = dim.Score.Value
		}
	}
	return scores
}
