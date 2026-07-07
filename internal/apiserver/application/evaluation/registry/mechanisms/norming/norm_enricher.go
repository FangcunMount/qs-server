package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ApplyNormProjection applies norm/T-score tables on top of raw scale scoring.
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
	if calcResult == nil || snapshot == nil || snapshot.Brief2 == nil {
		return calcResult
	}
	tables := snapshot.Brief2.NormTablesOrNil()
	if tables == nil {
		return calcResult
	}
	return calcnorm.Projection{
		Tables:               tables,
		Subject:              subject,
		PrimaryDimensionCode: snapshot.Brief2.PrimaryDimensionCode,
	}.Apply(calcResult)
}

// NormSubjectFromInput extracts norm lookup subject metadata from an input snapshot.
func NormSubjectFromInput(input *evaluationinput.InputSnapshot) calcnorm.Subject {
	if input == nil || input.NormSubject == nil {
		return calcnorm.Subject{}
	}
	return calcnorm.Subject{
		AgeMonths: input.NormSubject.AgeMonths,
		Gender:    input.NormSubject.Gender,
	}
}
