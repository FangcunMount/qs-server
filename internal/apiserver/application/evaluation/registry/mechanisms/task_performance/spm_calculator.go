package task_performance

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	calctask "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/task_performance"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

// CalculateSPM calculates Raven SPM from frozen answer keys. An unanswered
// item contributes zero; elapsed time is intentionally not enforced here.
// Pure scoring lives in domain/calculation/task_performance; this adapter
// maps InputSnapshot / SPMSpec into neutral inputs and Outcome Execution.
func CalculateSPM(input *portevaluationinput.InputSnapshot, snapshot *taskperfsnapshot.Snapshot) (*domainoutcome.Execution, error) {
	if input == nil || input.AnswerSheet == nil {
		return nil, fmt.Errorf("spm answer sheet is required")
	}
	if snapshot == nil || snapshot.SPM == nil {
		return nil, fmt.Errorf("spm execution spec is required")
	}
	modelRef := domainoutcome.ModelRef{ModelKind: modelcatalog.KindCognitive, ModelAlgorithm: modelcatalog.AlgorithmSPM, ModelCode: snapshot.Code, ModelVersion: snapshot.Version, ModelTitle: snapshot.Title}
	if input.Model != nil {
		modelRef.ModelKind = modelcatalog.Kind(input.Model.Kind)
		modelRef.ModelSubKind = modelcatalog.SubKind(input.Model.SubKind)
		modelRef.ModelAlgorithm = modelcatalog.Algorithm(input.Model.Algorithm)
		modelRef.ModelCode = input.Model.Code
		modelRef.ModelVersion = input.Model.Version
		modelRef.ModelTitle = input.Model.Title
	}
	execution := domainoutcome.NewExecution(modelRef, domainoutcome.Summary{}, domainoutcome.Detail{Kind: modelcatalog.KindCognitive})

	result := calctask.ScoreSPM(answersByQuestion(input.AnswerSheet.Answers), itemSetsFromSnapshot(snapshot.SPM.ItemSets), snapshot.SPM.TotalFactorCode)
	calculationadapter.MergeCalcResultIntoOutcome(execution, &result)
	if result.Primary != nil {
		total := result.Primary.Value
		execution.Summary.Score = &total
	}

	if snapshot.SPM.NormTables != nil {
		subject := calcnorm.Subject{}
		if input.NormSubject != nil {
			subject = calcnorm.Subject{AgeMonths: input.NormSubject.AgeMonths, Gender: input.NormSubject.Gender}
		}
		total := 0.0
		if result.Primary != nil {
			total = result.Primary.Value
		}
		if norm, ok := calcnorm.LookupNormScore(snapshot.SPM.NormTables, snapshot.SPM.TotalFactorCode, total, subject); ok {
			totalDimension := &execution.Dimensions[len(execution.Dimensions)-1]
			totalDimension.DerivedScores = append(totalDimension.DerivedScores, domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindPercentile, Value: norm.Percentile})
			scoreKind := domainoutcome.ScoreKindPercentile
			benchmark := 0.0
			if norm.StandardScore != nil {
				totalDimension.DerivedScores = append(totalDimension.DerivedScores, domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindStandardScore, Value: *norm.StandardScore})
				scoreKind = domainoutcome.ScoreKindStandardScore
			}
			totalDimension.NormReference = &domainoutcome.NormReference{
				ScoreKind:    scoreKind,
				Benchmark:    benchmark,
				TableVersion: snapshot.SPM.NormTables.NormTableVersion,
				FormVariant:  snapshot.SPM.NormTables.FormVariant,
				MinAgeMonths: norm.Reference.MinAgeMonths,
				MaxAgeMonths: norm.Reference.MaxAgeMonths,
				Gender:       norm.Reference.Gender,
			}
		}
	}
	return execution, nil
}

func itemSetsFromSnapshot(sets []taskperfsnapshot.SPMItemSet) []calctask.ItemSet {
	out := make([]calctask.ItemSet, 0, len(sets))
	for _, set := range sets {
		items := make([]calctask.Item, 0, len(set.Items))
		for _, item := range set.Items {
			items = append(items, calctask.Item{
				QuestionCode:      item.QuestionCode,
				CorrectOptionCode: item.CorrectOptionCode,
			})
		}
		out = append(out, calctask.ItemSet{Code: set.Code, Items: items})
	}
	return out
}

func answersByQuestion(answers []portevaluationinput.AnswerSnapshot) map[string]string {
	out := make(map[string]string, len(answers))
	for _, answer := range answers {
		if answer.QuestionCode == "" || answer.Value == nil {
			continue
		}
		out[answer.QuestionCode] = fmt.Sprint(answer.Value)
	}
	return out
}
