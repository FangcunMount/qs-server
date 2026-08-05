package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

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
