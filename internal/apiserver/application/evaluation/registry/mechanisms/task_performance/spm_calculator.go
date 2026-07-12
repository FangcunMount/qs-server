package task_performance

import (
	"fmt"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

// CalculateSPM calculates Raven SPM from frozen answer keys. An unanswered
// item contributes zero; elapsed time is intentionally not enforced here.
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
	answers := answersByQuestion(input.AnswerSheet.Answers)
	total := 0.0
	max := 0.0
	for _, set := range snapshot.SPM.ItemSets {
		setScore := 0.0
		setMax := float64(len(set.Items))
		for _, item := range set.Items {
			if answer, ok := answers[item.QuestionCode]; ok && answer == item.CorrectOptionCode {
				setScore++
			}
		}
		total += setScore
		max += setMax
		setMaxCopy := setMax
		execution.Dimensions = append(execution.Dimensions, domainoutcome.DimensionResult{
			Code: set.Code, Name: set.Code, Kind: domainoutcome.DimensionKindAbility, Role: "task_set",
			Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: setScore, Max: &setMaxCopy},
		})
	}
	maxCopy := max
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: total, Max: &maxCopy}
	execution.Summary.Score = &total
	execution.Dimensions = append(execution.Dimensions, domainoutcome.DimensionResult{
		Code: snapshot.SPM.TotalFactorCode, Name: snapshot.SPM.TotalFactorCode, Kind: domainoutcome.DimensionKindAbility, Role: "total",
		Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: total, Max: &maxCopy},
	})
	if snapshot.SPM.NormTables != nil {
		subject := calcnorm.Subject{}
		if input.NormSubject != nil {
			subject = calcnorm.Subject{AgeMonths: input.NormSubject.AgeMonths, Gender: input.NormSubject.Gender}
		}
		if norm, ok := calcnorm.LookupNormScore(snapshot.SPM.NormTables, snapshot.SPM.TotalFactorCode, total, subject); ok {
			totalDimension := &execution.Dimensions[len(execution.Dimensions)-1]
			totalDimension.DerivedScores = append(totalDimension.DerivedScores, domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindPercentile, Value: norm.Percentile})
			if norm.StandardScore != nil {
				totalDimension.DerivedScores = append(totalDimension.DerivedScores, domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindStandardScore, Value: *norm.StandardScore})
			}
		}
	}
	return execution, nil
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
