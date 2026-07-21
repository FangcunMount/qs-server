package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

// ApplyNormProjection 应用常模/T 分 tables 基于 原始 scale 计分。
func ApplyNormProjection(
	outcome *domainoutcome.Execution,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) (*domainoutcome.Execution, error) {
	if outcome == nil || snapshot == nil {
		return outcome, nil
	}
	calcResult, err := enrichNormCalcResult(calculationadapter.CalcResultFromOutcome(outcome), snapshot, subject)
	if err != nil {
		return nil, err
	}
	return calculationadapter.MergeCalcResultIntoOutcome(outcome, calcResult), nil
}

func enrichNormCalcResult(
	calcResult *calculation.Result,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) (*calculation.Result, error) {
	if calcResult == nil || snapshot == nil || snapshot.Norming == nil {
		return calcResult, nil
	}
	tables := snapshot.Norming.NormTablesOrNil()
	return calcnorm.Projection{
		Tables:               tables,
		Subject:              subject,
		PrimaryDimensionCode: snapshot.Norming.PrimaryDimensionCode,
		RequiredFactorCodes:  snapshot.Norming.RequiredFactorCodes,
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
