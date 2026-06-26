package service

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestQuestionnaireServiceGetQuestionnaireByVersion(t *testing.T) {
	query := &fakeQuestionnaireQueryService{
		byVersion: &questionnaire.QuestionnaireResult{
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
			Type:    "Survey",
			Status:  "published",
		},
	}
	svc := NewQuestionnaireService(query, &fakePublishedReader{
		snapshot: &domain.Snapshot{},
	})

	resp, err := svc.GetQuestionnaire(context.Background(), &pb.GetQuestionnaireRequest{
		Code:    "MBTI_OEJTS",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetQuestionnaire() error = %v", err)
	}
	if resp.GetQuestionnaire().GetVersion() != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", resp.GetQuestionnaire().GetVersion())
	}
	if !query.byVersionCalled {
		t.Fatal("GetPublishedByCodeVersion was not called")
	}
}

func TestQuestionnaireServiceGetQuestionnaireRejectsUnboundSurvey(t *testing.T) {
	query := &fakeQuestionnaireQueryService{
		byVersion: &questionnaire.QuestionnaireResult{
			Code:    "SURVEY_X",
			Version: "1.0.0",
			Type:    "Survey",
			Status:  "published",
		},
	}
	svc := NewQuestionnaireService(query, &fakePublishedReader{})

	_, err := svc.GetQuestionnaire(context.Background(), &pb.GetQuestionnaireRequest{
		Code:    "SURVEY_X",
		Version: "1.0.0",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

type fakeQuestionnaireQueryService struct {
	byCode          *questionnaire.QuestionnaireResult
	byVersion       *questionnaire.QuestionnaireResult
	byVersionCalled bool
}

func (s *fakeQuestionnaireQueryService) GetByCode(context.Context, string) (*questionnaire.QuestionnaireResult, error) {
	return nil, nil
}

func (s *fakeQuestionnaireQueryService) List(context.Context, questionnaire.ListQuestionnairesDTO) (*questionnaire.QuestionnaireSummaryListResult, error) {
	return nil, nil
}

func (s *fakeQuestionnaireQueryService) GetPublishedByCode(context.Context, string) (*questionnaire.QuestionnaireResult, error) {
	return s.byCode, nil
}

func (s *fakeQuestionnaireQueryService) GetPublishedByCodeVersion(_ context.Context, code, version string) (*questionnaire.QuestionnaireResult, error) {
	s.byVersionCalled = true
	if s.byVersion == nil {
		return nil, nil
	}
	if s.byVersion.Code != code || s.byVersion.Version != version {
		return nil, nil
	}
	return s.byVersion, nil
}

func (s *fakeQuestionnaireQueryService) GetQuestionCount(context.Context, string) (int32, error) {
	return 0, nil
}

func (s *fakeQuestionnaireQueryService) ListPublished(context.Context, questionnaire.ListQuestionnairesDTO) (*questionnaire.QuestionnaireSummaryListResult, error) {
	return nil, nil
}

type fakePublishedReader struct {
	snapshot *domain.Snapshot
}

func (r *fakePublishedReader) GetPublishedByRef(context.Context, rulesetport.Ref) (*domain.Snapshot, error) {
	return nil, nil
}

func (r *fakePublishedReader) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.Snapshot, error) {
	return r.snapshot, nil
}
