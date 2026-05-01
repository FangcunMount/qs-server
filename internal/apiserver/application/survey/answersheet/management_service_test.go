package answersheet

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	questionnairedomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
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
	filter := buildListFilter(ListAnswerSheetsDTO{
		QuestionnaireCode: "QNR-001",
		FillerID:          &fillerID,
		StartTime:         &startTime,
		EndTime:           &endTime,
	})

	if got := filter.QuestionnaireCode; got != "QNR-001" {
		t.Fatalf("questionnaire_code = %v, want QNR-001", got)
	}
	if filter.FillerID == nil || *filter.FillerID != fillerID {
		t.Fatalf("filler_id = %v, want %d", filter.FillerID, fillerID)
	}
	if got := filter.StartTime; got != &startTime {
		t.Fatalf("start_time = %v, want %v", got, &startTime)
	}
	if got := filter.EndTime; got != &endTime {
		t.Fatalf("end_time = %v, want %v", got, &endTime)
	}
}

func TestManagementServiceListUsesReadModelFilter(t *testing.T) {
	t.Parallel()

	fillerID := uint64(9)
	startTime := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	reader := &answerSheetReaderStub{}
	service := &managementService{
		repo:   &managementRepoStub{},
		reader: reader,
	}

	_, err := service.List(context.Background(), ListAnswerSheetsDTO{
		QuestionnaireCode: "QNR-009",
		FillerID:          &fillerID,
		StartTime:         &startTime,
		Page:              1,
		PageSize:          20,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if reader.listFilter.QuestionnaireCode != "QNR-009" || reader.listFilter.FillerID == nil || *reader.listFilter.FillerID != fillerID {
		t.Fatalf("captured list filter = %#v", reader.listFilter)
	}
	if reader.countFilter.QuestionnaireCode != "QNR-009" || reader.countFilter.FillerID == nil || *reader.countFilter.FillerID != fillerID {
		t.Fatalf("captured count filter = %#v", reader.countFilter)
	}
}

type answerSheetReaderStub struct {
	listFilter  surveyreadmodel.AnswerSheetFilter
	countFilter surveyreadmodel.AnswerSheetFilter
}

func (s *answerSheetReaderStub) ListAnswerSheets(_ context.Context, filter surveyreadmodel.AnswerSheetFilter, _ surveyreadmodel.PageRequest) ([]surveyreadmodel.AnswerSheetSummaryRow, error) {
	s.listFilter = filter
	return []surveyreadmodel.AnswerSheetSummaryRow{}, nil
}

func (s *answerSheetReaderStub) CountAnswerSheets(_ context.Context, filter surveyreadmodel.AnswerSheetFilter) (int64, error) {
	s.countFilter = filter
	return 0, nil
}

func TestManagementServiceGetByIDReturnsConvertedAnswerSheet(t *testing.T) {
	t.Parallel()

	answer1, _ := domainanswersheet.NewAnswer(meta.NewCode("q1"), questionnairedomain.TypeRadio, domainanswersheet.NewOptionValue("A"), 1.5)
	answer2, _ := domainanswersheet.NewAnswer(meta.NewCode("q2"), questionnairedomain.TypeNumber, domainanswersheet.NewNumberValue(7), 2.5)
	sheet, err := domainanswersheet.NewAnswerSheet(
		domainanswersheet.NewQuestionnaireRef("QNR-001", "v1", "PHQ-9"),
		actor.NewFillerRef(7, actor.FillerTypeGuardian),
		[]domainanswersheet.Answer{answer1, answer2},
		time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewAnswerSheet() error = %v", err)
	}
	sheet.AssignID(meta.FromUint64(12))

	service := &managementService{
		repo: &managementRepoStub{
			findByIDFunc: func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error) {
				return sheet, nil
			},
		},
	}

	result, err := service.GetByID(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if result.ID != 12 || result.QuestionnaireCode != "QNR-001" || result.QuestionnaireVer != "v1" {
		t.Fatalf("unexpected identity fields: %+v", result)
	}
	if result.FillerID != 7 || len(result.Answers) != 2 {
		t.Fatalf("unexpected filler/answers: %+v", result)
	}
	if result.Answers[0].QuestionCode != "q1" || result.Answers[1].QuestionCode != "q2" {
		t.Fatalf("unexpected answers: %+v", result.Answers)
	}
}

func TestManagementServiceDeleteDelegatesToRepository(t *testing.T) {
	t.Parallel()

	var deletedID meta.ID
	service := &managementService{
		repo: &managementRepoStub{
			findByIDFunc: func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error) {
				return &domainanswersheet.AnswerSheet{}, nil
			},
			deleteFunc: func(_ context.Context, id meta.ID) error {
				deletedID = id
				return nil
			},
		},
	}

	if err := service.Delete(context.Background(), 9); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if deletedID != meta.FromUint64(9) {
		t.Fatalf("deletedID = %d, want 9", deletedID)
	}
}

func TestAnswerSheetIDConvertersRejectOverflow(t *testing.T) {
	t.Parallel()

	if _, err := answerSheetIDFromUint64("answersheet_id", math.MaxUint64); err == nil {
		t.Fatal("answerSheetIDFromUint64 expected overflow error")
	}
	if _, err := fillerUserIDFromUint64("filler_id", math.MaxUint64); err == nil {
		t.Fatal("fillerUserIDFromUint64 expected overflow error")
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("mustUint64FromInt64 expected panic on negative input")
		}
	}()
	_ = mustUint64FromInt64("filler_id", -1)
}

func TestManagementServiceDeleteWrapsMissingAnswerSheet(t *testing.T) {
	t.Parallel()

	service := &managementService{
		repo: &managementRepoStub{
			findByIDFunc: func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error) {
				return nil, errors.WithCode(errorCode.ErrAnswerSheetNotFound, "missing")
			},
		},
	}

	err := service.Delete(context.Background(), 7)
	if err == nil {
		t.Fatal("Delete expected error")
	}
	if code := errors.ParseCoder(err).Code(); code != errorCode.ErrAnswerSheetNotFound {
		t.Fatalf("error code = %d, want %d", code, errorCode.ErrAnswerSheetNotFound)
	}
}
