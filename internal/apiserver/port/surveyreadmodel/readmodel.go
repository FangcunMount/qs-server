package surveyreadmodel

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// PageRequest describes a read-model page request.
type PageRequest struct {
	Page     int
	PageSize int
}

// QuestionnaireFilter contains typed filters for questionnaire list queries.
type QuestionnaireFilter struct {
	Status string
	Title  string
	Type   string
}

// IsEmpty reports whether no optional questionnaire filters were supplied.
func (f QuestionnaireFilter) IsEmpty() bool {
	return f.Status == "" && f.Title == "" && f.Type == ""
}

// QuestionnaireSummaryRow is a transport-neutral questionnaire list row.
type QuestionnaireSummaryRow struct {
	Code          string
	Version       string
	Title         string
	Description   string
	ImgURL        string
	Status        string
	Type          string
	QuestionCount int
	CreatedBy     meta.ID
	CreatedAt     time.Time
	UpdatedBy     meta.ID
	UpdatedAt     time.Time
}

// QuestionnaireReader exposes questionnaire read-model queries.
type QuestionnaireReader interface {
	ListQuestionnaires(ctx context.Context, filter QuestionnaireFilter, page PageRequest) ([]QuestionnaireSummaryRow, error)
	CountQuestionnaires(ctx context.Context, filter QuestionnaireFilter) (int64, error)
	ListPublishedQuestionnaires(ctx context.Context, filter QuestionnaireFilter, page PageRequest) ([]QuestionnaireSummaryRow, error)
	CountPublishedQuestionnaires(ctx context.Context, filter QuestionnaireFilter) (int64, error)
}

// AnswerSheetFilter contains typed filters for answer-sheet list queries.
type AnswerSheetFilter struct {
	QuestionnaireCode string
	FillerID          *uint64
	StartTime         *time.Time
	EndTime           *time.Time
}

// AnswerSheetSummaryRow is a transport-neutral answer-sheet list row.
type AnswerSheetSummaryRow struct {
	ID                   meta.ID
	QuestionnaireCode    string
	QuestionnaireVersion string
	QuestionnaireTitle   string
	FillerID             uint64
	FillerType           string
	TotalScore           float64
	AnswerCount          int
	FilledAt             time.Time
}

// AnswerSheetReader exposes answer-sheet read-model queries.
type AnswerSheetReader interface {
	ListAnswerSheets(ctx context.Context, filter AnswerSheetFilter, page PageRequest) ([]AnswerSheetSummaryRow, error)
	CountAnswerSheets(ctx context.Context, filter AnswerSheetFilter) (int64, error)
}
