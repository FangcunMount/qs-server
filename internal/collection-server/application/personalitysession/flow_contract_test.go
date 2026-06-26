package personalitysession

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
)

// flowModelReader returns a published MBTI-like model binding.
type flowModelReader struct{}

func (flowModelReader) Get(_ context.Context, code string) (*personalitymodel.PersonalityModelResponse, error) {
	if code != "MBTI_OEJTS" {
		return nil, nil
	}
	return &personalitymodel.PersonalityModelResponse{
		Code:                 "MBTI_OEJTS",
		Version:              "1.0.0",
		Title:                "MBTI OEJTS",
		Algorithm:            "mbti",
		QuestionnaireCode:    "MBTI_OEJTS",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		QuestionCount:        93,
	}, nil
}

// flowQuestionnaireReader returns the bound questionnaire version.
type flowQuestionnaireReader struct{}

func (flowQuestionnaireReader) Get(_ context.Context, code, version string) (*questionnaire.QuestionnaireResponse, error) {
	if code != "MBTI_OEJTS" || version != "1.0.0" {
		return nil, nil
	}
	return &questionnaire.QuestionnaireResponse{
		Code:    code,
		Version: version,
		Title:   "MBTI OEJTS",
		Status:  "published",
		Type:    "Survey",
		Questions: []questionnaire.QuestionResponse{
			{Code: "Q1", Type: "Radio", Title: "sample"},
		},
	}, nil
}

// flowAssessmentReader simulates post-submit assessment lookup and report fetch.
type flowAssessmentReader struct {
	answerSheetID uint64
	assessmentID  uint64
	testeeID      uint64
}

func (f *flowAssessmentReader) GetByAnswerSheet(_ context.Context, answerSheetID uint64) (uint64, string, error) {
	if answerSheetID != f.answerSheetID {
		return 0, "", nil
	}
	return f.assessmentID, "interpreted", nil
}

func (f *flowAssessmentReader) GetReport(_ context.Context, testeeID, assessmentID uint64) (bool, error) {
	if testeeID != f.testeeID || assessmentID != f.assessmentID {
		return false, nil
	}
	return true, nil
}

func TestMiniProgramPersonalityAssessmentFlowContract(t *testing.T) {
	const testeeID uint64 = 1001
	const answerSheetID uint64 = 9001
	const assessmentID uint64 = 8001

	sessionSvc := NewService(flowModelReader{}, flowQuestionnaireReader{})
	session, err := sessionSvc.Start(context.Background(), &StartSessionRequest{
		ModelCode: "MBTI_OEJTS",
		TesteeID:  testeeID,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if session == nil {
		t.Fatal("Start() returned nil session")
	}

	if session.SubmitContract.QuestionnaireCode != "MBTI_OEJTS" ||
		session.SubmitContract.QuestionnaireVersion != "1.0.0" ||
		session.SubmitContract.TesteeID != strconv.FormatUint(testeeID, 10) {
		t.Fatalf("unexpected submit contract: %#v", session.SubmitContract)
	}
	if session.Questionnaire.Version != session.SubmitContract.QuestionnaireVersion {
		t.Fatalf("questionnaire version drift: questionnaire=%q contract=%q",
			session.Questionnaire.Version, session.SubmitContract.QuestionnaireVersion)
	}
	if !strings.Contains(session.Endpoints.Report, "testee_id="+strconv.FormatUint(testeeID, 10)) {
		t.Fatalf("report endpoint missing testee_id: %q", session.Endpoints.Report)
	}
	if session.Endpoints.SubmitAnswerSheet != "/api/v1/answersheets" {
		t.Fatalf("submit endpoint = %q", session.Endpoints.SubmitAnswerSheet)
	}

	reader := &flowAssessmentReader{
		answerSheetID: answerSheetID,
		assessmentID:  assessmentID,
		testeeID:      testeeID,
	}
	gotAssessmentID, status, err := reader.GetByAnswerSheet(context.Background(), answerSheetID)
	if err != nil {
		t.Fatalf("GetByAnswerSheet() error = %v", err)
	}
	if gotAssessmentID != assessmentID || status != "interpreted" {
		t.Fatalf("assessment lookup = (%d, %q), want (%d, interpreted)", gotAssessmentID, status, assessmentID)
	}

	ok, err := reader.GetReport(context.Background(), testeeID, assessmentID)
	if err != nil {
		t.Fatalf("GetReport() error = %v", err)
	}
	if !ok {
		t.Fatal("expected report for owner testee")
	}
	ok, err = reader.GetReport(context.Background(), testeeID+1, assessmentID)
	if err != nil {
		t.Fatalf("GetReport(wrong testee) error = %v", err)
	}
	if ok {
		t.Fatal("expected report access denied for wrong testee")
	}
}

func TestMiniProgramPersonalityAssessmentFlowRejectsUnknownModel(t *testing.T) {
	sessionSvc := NewService(flowModelReader{}, flowQuestionnaireReader{})
	session, err := sessionSvc.Start(context.Background(), &StartSessionRequest{
		ModelCode: "UNKNOWN_MODEL",
		TesteeID:  1,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if session != nil {
		t.Fatalf("Start() = %#v, want nil", session)
	}
}
