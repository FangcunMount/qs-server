package answersheet

import (
	"context"
	"testing"
	"time"

	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type managementRepoStub struct {
	findByIDFunc                   func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error)
	findSummaryListByQuestionnaire func(context.Context, string, int, int) ([]*domainanswersheet.AnswerSheetSummary, error)
	countWithConditionsFunc        func(context.Context, map[string]interface{}) (int64, error)
	deleteFunc                     func(context.Context, meta.ID) error
}

func (s *managementRepoStub) Create(context.Context, *domainanswersheet.AnswerSheet) error {
	return nil
}

func (s *managementRepoStub) Update(context.Context, *domainanswersheet.AnswerSheet) error {
	return nil
}

func (s *managementRepoStub) FindByID(ctx context.Context, id meta.ID) (*domainanswersheet.AnswerSheet, error) {
	if s.findByIDFunc != nil {
		return s.findByIDFunc(ctx, id)
	}
	return nil, nil
}

func (s *managementRepoStub) FindSummaryListByFiller(context.Context, uint64, int, int) ([]*domainanswersheet.AnswerSheetSummary, error) {
	return nil, nil
}

func (s *managementRepoStub) FindSummaryListByQuestionnaire(ctx context.Context, code string, page, pageSize int) ([]*domainanswersheet.AnswerSheetSummary, error) {
	if s.findSummaryListByQuestionnaire != nil {
		return s.findSummaryListByQuestionnaire(ctx, code, page, pageSize)
	}
	return nil, nil
}

func (s *managementRepoStub) CountByFiller(context.Context, uint64) (int64, error) { return 0, nil }

func (s *managementRepoStub) CountByQuestionnaire(context.Context, string) (int64, error) {
	return 0, nil
}

func (s *managementRepoStub) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	if s.countWithConditionsFunc != nil {
		return s.countWithConditionsFunc(ctx, conditions)
	}
	return 0, nil
}

func (s *managementRepoStub) Delete(ctx context.Context, id meta.ID) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, id)
	}
	return nil
}

func TestValidateManagementListDTORejectsInvalidPaging(t *testing.T) {
	t.Parallel()

	cases := []ListAnswerSheetsDTO{
		{Page: 0, PageSize: 10},
		{Page: 1, PageSize: 0},
		{Page: 1, PageSize: 101},
	}

	for _, dto := range cases {
		if err := validateManagementListDTO(dto); err == nil {
			t.Fatalf("validateManagementListDTO(%+v) expected error", dto)
		}
	}
}

func TestBuildListConditionsIncludesOptionalFilters(t *testing.T) {
	t.Parallel()

	fillerID := uint64(42)
	startTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(24 * time.Hour)
	conditions := buildListConditions(ListAnswerSheetsDTO{
		QuestionnaireCode: "QNR-001",
		FillerID:          &fillerID,
		StartTime:         &startTime,
		EndTime:           &endTime,
		Conditions: map[string]string{
			"status": "completed",
		},
	})

	if got := conditions["questionnaire_code"]; got != "QNR-001" {
		t.Fatalf("questionnaire_code = %v, want QNR-001", got)
	}
	if got := conditions["filler_id"]; got != fillerID {
		t.Fatalf("filler_id = %v, want %d", got, fillerID)
	}
	if got := conditions["status"]; got != "completed" {
		t.Fatalf("status = %v, want completed", got)
	}
	if got := conditions["start_time"]; got != &startTime {
		t.Fatalf("start_time = %v, want %v", got, &startTime)
	}
	if got := conditions["end_time"]; got != &endTime {
		t.Fatalf("end_time = %v, want %v", got, &endTime)
	}
}

func TestManagementServiceListUsesBuiltConditions(t *testing.T) {
	t.Parallel()

	fillerID := uint64(9)
	startTime := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	var captured map[string]interface{}
	service := &managementService{
		repo: &managementRepoStub{
			findSummaryListByQuestionnaire: func(context.Context, string, int, int) ([]*domainanswersheet.AnswerSheetSummary, error) {
				return []*domainanswersheet.AnswerSheetSummary{}, nil
			},
			countWithConditionsFunc: func(_ context.Context, conditions map[string]interface{}) (int64, error) {
				captured = conditions
				return 0, nil
			},
		},
	}

	_, err := service.List(context.Background(), ListAnswerSheetsDTO{
		QuestionnaireCode: "QNR-009",
		FillerID:          &fillerID,
		StartTime:         &startTime,
		Page:              1,
		PageSize:          20,
		Conditions: map[string]string{
			"kind": "manual",
		},
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if captured["questionnaire_code"] != "QNR-009" || captured["filler_id"] != fillerID || captured["kind"] != "manual" {
		t.Fatalf("captured conditions = %#v", captured)
	}
}
