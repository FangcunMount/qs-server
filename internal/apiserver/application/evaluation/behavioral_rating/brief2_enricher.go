package behavioralrating

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// EnrichBrief2Outcome applies Brief-2 norm/T-score projection on top of raw scale scoring.
func EnrichBrief2Outcome(
	outcome *assessment.AssessmentOutcome,
	snapshot *behavioralsnapshot.Snapshot,
	subject brief2norm.Subject,
) *assessment.AssessmentOutcome {
	if outcome == nil || snapshot == nil {
		return outcome
	}
	calcResult := enrichBrief2CalcResult(calcResultFromOutcome(outcome), snapshot, subject)
	return mergeCalcResultIntoOutcome(outcome, calcResult)
}

func enrichBrief2CalcResult(
	calcResult *calculation.Result,
	snapshot *behavioralsnapshot.Snapshot,
	subject brief2norm.Subject,
) *calculation.Result {
	if calcResult == nil || snapshot == nil || snapshot.Brief2 == nil {
		return calcResult
	}
	tables := snapshot.Brief2.NormTablesOrNil()
	if tables == nil {
		return calcResult
	}
	return brief2NormProjection{
		tables:               tables,
		subject:              subject,
		primaryDimensionCode: snapshot.Brief2.PrimaryDimensionCode,
	}.apply(calcResult)
}

func NormSubjectFromInput(input *evaluationinput.InputSnapshot) brief2norm.Subject {
	if input == nil || input.NormSubject == nil {
		return brief2norm.Subject{}
	}
	return brief2norm.Subject{
		AgeMonths: input.NormSubject.AgeMonths,
		Gender:    input.NormSubject.Gender,
	}
}
