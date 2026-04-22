package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/gin-gonic/gin"
)

func TestBuildAnswerSheetListDTOParsesFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("GET", "/api/v1/answersheets?page=2&page_size=30&questionnaire_code=QNR-001&filler_id=42&start_time=2026-04-01&end_time=2026-04-02", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	dto, err := buildAnswerSheetListDTO(c)
	if err != nil {
		t.Fatalf("buildAnswerSheetListDTO returned error: %v", err)
	}
	if dto.Page != 2 || dto.PageSize != 30 || dto.QuestionnaireCode != "QNR-001" {
		t.Fatalf("unexpected dto: %+v", dto)
	}
	if dto.FillerID == nil || *dto.FillerID != 42 {
		t.Fatalf("filler_id = %+v, want 42", dto.FillerID)
	}
	if dto.StartTime == nil || dto.StartTime.Format("2006-01-02") != "2026-04-01" {
		t.Fatalf("start_time = %+v", dto.StartTime)
	}
	if dto.EndTime == nil || dto.EndTime.Format("2006-01-02") != "2026-04-02" {
		t.Fatalf("end_time = %+v", dto.EndTime)
	}
}

func TestBuildAnswerSheetListDTORejectsInvalidPage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("GET", "/api/v1/answersheets?page=0", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if _, err := buildAnswerSheetListDTO(c); err == nil {
		t.Fatal("expected invalid page error")
	}
}

func TestResolveAdminSubmitFillerIDPrefersExplicitFieldsThenContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &AnswerSheetHandler{}
	handler.BaseHandler = *NewBaseHandler()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if got, ok := handler.resolveAdminSubmitFillerID(c, request.AdminSubmitAnswerSheetRequest{FillerID: 11, WriterID: 12}); !ok || got != 11 {
		t.Fatalf("filler priority mismatch: got %d, ok=%v", got, ok)
	}
	if got, ok := handler.resolveAdminSubmitFillerID(c, request.AdminSubmitAnswerSheetRequest{WriterID: 12}); !ok || got != 12 {
		t.Fatalf("writer fallback mismatch: got %d, ok=%v", got, ok)
	}
	c.Set(middleware.UserIDKey, uint64(13))
	if got, ok := handler.resolveAdminSubmitFillerID(c, request.AdminSubmitAnswerSheetRequest{}); !ok || got != 13 {
		t.Fatalf("context fallback mismatch: got %d, ok=%v", got, ok)
	}
}

func TestBuildAdminSubmitDTOPreservesAnswerPayload(t *testing.T) {
	t.Parallel()

	dto := buildAdminSubmitDTO(request.AdminSubmitAnswerSheetRequest{
		QuestionnaireCode:    "QNR-002",
		QuestionnaireVersion: "v2",
		TesteeID:             21,
		TaskID:               "task-1",
		Answers: []request.AdminAnswerSubmit{
			{QuestionCode: "q1", QuestionType: "Radio", Value: "A"},
			{QuestionCode: "q2", QuestionType: "Number", Value: 12},
		},
	}, 31, 41)

	if dto.FillerID != 31 || dto.OrgID != 41 || dto.TesteeID != 21 {
		t.Fatalf("unexpected dto identity fields: %+v", dto)
	}
	if len(dto.Answers) != 2 || dto.Answers[1].Value != 12 {
		t.Fatalf("unexpected answers: %+v", dto.Answers)
	}
}

func TestOptionalDateQueryReturnsNilForInvalidDate(t *testing.T) {
	t.Parallel()

	if got := optionalDateQuery("2026-13-99"); got != nil {
		t.Fatalf("expected nil for invalid date, got %v", got)
	}
	if got := optionalDateQuery("2026-04-22"); got == nil || !got.Equal(time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected parsed date: %v", got)
	}
}
