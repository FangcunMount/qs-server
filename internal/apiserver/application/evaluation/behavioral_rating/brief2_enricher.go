package behavioralrating

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/projection"
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
	if outcome == nil || snapshot == nil || snapshot.Brief2 == nil {
		return outcome
	}
	tables := snapshot.Brief2.NormTablesOrNil()
	if tables == nil {
		return outcome
	}
	return projection.Brief2NormProjection{Tables: tables, Subject: subject}.Apply(outcome)
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
