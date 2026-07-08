package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ApplyNormProjection 应用常模/T 分 tables 基于 原始 scale 计分。
func ApplyNormProjection(
	outcome *assessment.AssessmentOutcome,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) *assessment.AssessmentOutcome {
	if outcome == nil || snapshot == nil {
		return outcome
	}
	calcResult := enrichNormCalcResult(calculationadapter.CalcResultFromOutcome(outcome), snapshot, subject)
	return calculationadapter.MergeCalcResultIntoOutcome(outcome, calcResult)
}

func enrichNormCalcResult(
	calcResult *calculation.Result,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) *calculation.Result {
	if calcResult == nil || snapshot == nil || snapshot.Norming == nil {
		return calcResult
	}
	tables := snapshot.Norming.NormTablesOrNil()
	if tables == nil {
		return calcResult
	}
	return calcnorm.Projection{
		Tables:               tables,
		Subject:              subject,
		PrimaryDimensionCode: snapshot.Norming.PrimaryDimensionCode,
	}.Apply(calcResult)
}

// NormSubjectFromInput extracts 常模 lookup subject 元数据 从 input 快照。
func NormSubjectFromInput(input *evaluationinput.InputSnapshot) calcnorm.Subject {
	if input == nil || input.NormSubject == nil {
		return calcnorm.Subject{}
	}
	return calcnorm.Subject{
		AgeMonths: input.NormSubject.AgeMonths,
		Gender:    input.NormSubject.Gender,
	}
}
