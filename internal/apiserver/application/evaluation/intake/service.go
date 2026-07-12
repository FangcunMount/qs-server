// Package intake contains the Evaluation capability used by answer-sheet
// orchestration. Cross-module sequencing belongs to a Journey, not here.
package intake

import (
	"context"

	legacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
)

type CreateCommand = legacy.CreateAssessmentDTO
type Assessment = legacy.AssessmentResult

type Service interface {
	CreateForAnswerSheet(context.Context, CreateCommand) (*Assessment, error)
	SubmitForEvaluation(context.Context, uint64) (*Assessment, error)
	FindByAnswerSheetID(context.Context, uint64) (*Assessment, error)
}

func Adapt(service legacy.AnswerSheetAssessmentIntakeService) Service { return service }
