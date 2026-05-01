package answersheet

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAnswerSheetFilterToBSONMapsTypedFilter(t *testing.T) {
	t.Parallel()

	fillerID := uint64(1001)
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(time.Hour)
	query := answerSheetFilterToBSON(surveyreadmodel.AnswerSheetFilter{
		QuestionnaireCode: "Q_A",
		FillerID:          &fillerID,
		StartTime:         &start,
		EndTime:           &end,
	})

	if got := query["questionnaire_code"]; got != "Q_A" {
		t.Fatalf("questionnaire_code = %#v, want Q_A", got)
	}
	if got := query["filler_id"]; got != fillerID {
		t.Fatalf("filler_id = %#v, want %d", got, fillerID)
	}
	if got := query["start_time"]; got != &start {
		t.Fatalf("start_time = %#v, want start pointer", got)
	}
	if got := query["end_time"]; got != &end {
		t.Fatalf("end_time = %#v, want end pointer", got)
	}
	if got := query["deleted_at"]; got != nil {
		t.Fatalf("deleted_at = %#v, want nil", got)
	}
}

func TestAnswerSheetFilterToBSONOmitsNonPositiveFillerID(t *testing.T) {
	t.Parallel()

	fillerID := uint64(0)
	query := answerSheetFilterToBSON(surveyreadmodel.AnswerSheetFilter{FillerID: &fillerID})
	if _, ok := query["filler_id"]; ok {
		t.Fatalf("filler_id should be omitted for zero value, got %#v", query["filler_id"])
	}
}

func TestAnswerSheetRowFromPOMapsSummaryProjection(t *testing.T) {
	t.Parallel()

	filledAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	id := meta.New()
	row, err := answerSheetRowFromPO(&AnswerSheetSummaryPO{
		DomainID:           id.Uint64(),
		QuestionnaireCode:  "Q_A",
		QuestionnaireTitle: "Questionnaire A",
		FillerID:           1001,
		FillerType:         "testee",
		TotalScore:         12.5,
		AnswerCount:        7,
		FilledAt:           &filledAt,
	})
	if err != nil {
		t.Fatalf("answerSheetRowFromPO() error = %v", err)
	}

	if row.ID.Uint64() != id.Uint64() {
		t.Fatalf("id = %d, want %d", row.ID.Uint64(), id.Uint64())
	}
	if row.QuestionnaireCode != "Q_A" || row.QuestionnaireTitle != "Questionnaire A" {
		t.Fatalf("questionnaire fields = (%q,%q), want (Q_A,Questionnaire A)", row.QuestionnaireCode, row.QuestionnaireTitle)
	}
	if row.FillerID != 1001 || row.FillerType != "testee" || row.TotalScore != 12.5 || row.AnswerCount != 7 {
		t.Fatalf("row summary fields = %#v", row)
	}
	if !row.FilledAt.Equal(filledAt) {
		t.Fatalf("filled_at = %s, want %s", row.FilledAt, filledAt)
	}
}
