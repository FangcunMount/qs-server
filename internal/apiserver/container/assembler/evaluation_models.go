package assembler

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationResult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	scaleEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
)

type evaluationModelRegistration struct {
	key              evaluation.EvaluatorKey
	newEvaluator     func() execute.Evaluator
	newReportBuilder func() evaluationResult.ReportBuilder
}

func defaultEvaluationModelRegistrations(
	scaleReportBuilder report.ReportBuilder,
) []evaluationModelRegistration {
	scaleScorer := ruleengine.NewScaleFactorScorer()
	return []evaluationModelRegistration{
		{
			key: evaluation.EvaluatorKeyScaleDefault,
			newEvaluator: func() execute.Evaluator {
				return scaleEvaluation.NewExecutor(scaleScorer)
			},
			newReportBuilder: func() evaluationResult.ReportBuilder {
				return evaluationResult.NewScaleReportBuilder(scaleReportBuilder)
			},
		},
		{
			key: evaluation.PersonalityTypologyKey(assessmentmodel.AlgorithmMBTI),
			newEvaluator: func() execute.Evaluator {
				return typologyEvaluation.NewMBTIExecutor()
			},
			newReportBuilder: func() evaluationResult.ReportBuilder {
				return typologyEvaluation.NewMBTIReportBuilder()
			},
		},
		{
			key: evaluation.PersonalityTypologyKey(assessmentmodel.AlgorithmSBTI),
			newEvaluator: func() execute.Evaluator {
				return typologyEvaluation.NewSBTIExecutor()
			},
			newReportBuilder: func() evaluationResult.ReportBuilder {
				return typologyEvaluation.NewSBTIReportBuilder()
			},
		},
	}
}

func buildEvaluators(regs []evaluationModelRegistration) ([]execute.Evaluator, error) {
	evaluators := make([]execute.Evaluator, 0, len(regs))
	for _, reg := range regs {
		if reg.newEvaluator == nil {
			continue
		}
		evaluators = append(evaluators, reg.newEvaluator())
	}
	return evaluators, nil
}

func buildReportBuilders(regs []evaluationModelRegistration) ([]evaluationResult.ReportBuilder, error) {
	builders := make([]evaluationResult.ReportBuilder, 0, len(regs))
	for _, reg := range regs {
		if reg.newReportBuilder == nil {
			continue
		}
		builders = append(builders, reg.newReportBuilder())
	}
	return builders, nil
}
