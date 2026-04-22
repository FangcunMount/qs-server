package answersheet

import (
	"context"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	questionnairedomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestValidateSubmitDTORejectsMissingFields(t *testing.T) {
	t.Parallel()

	service := &submissionService{}
	log := logger.L(context.Background())
	cases := []SubmitAnswerSheetDTO{
		{},
		{QuestionnaireCode: "QNR-001"},
		{QuestionnaireCode: "QNR-001", FillerID: 1},
		{QuestionnaireCode: "QNR-001", FillerID: 1, TesteeID: 2},
		{
			QuestionnaireCode: "QNR-001",
			FillerID:          1,
			TesteeID:          2,
			Answers: []AnswerDTO{
				{QuestionType: "Radio", Value: "A"},
			},
		},
	}

	for _, dto := range cases {
		if err := service.validateSubmitDTO(log, dto); err == nil {
			t.Fatalf("validateSubmitDTO(%+v) expected error", dto)
		}
	}
}

func TestValidateAnswersBatchReturnsQuestionDetails(t *testing.T) {
	t.Parallel()

	service := &submissionService{batchValidator: validation.NewBatchValidator()}
	log := logger.L(context.Background())

	err := service.validateAnswersBatch(log, []validation.ValidationTask{
		{
			ID:    "q1",
			Value: domainanswersheet.NewAnswerValueAdapter(domainanswersheet.NewStringValue("")),
			Rules: []validation.ValidationRule{
				validation.NewValidationRule(validation.RuleTypeRequired, "true"),
			},
		},
	})
	if err == nil {
		t.Fatal("validateAnswersBatch expected error")
	}
	if coder := pkgerrors.ParseCoder(err); coder.Code() != errorCode.ErrAnswerSheetInvalid {
		t.Fatalf("error code = %d, want %d", coder.Code(), errorCode.ErrAnswerSheetInvalid)
	}
}

func TestCreateAnswersBuildsDomainAnswers(t *testing.T) {
	t.Parallel()

	service := &submissionService{}
	log := logger.L(context.Background())
	answers, err := service.createAnswers(log, []answerBuildResult{
		{
			questionCode: "q1",
			questionType: questionnairedomain.TypeRadio,
			answerValue:  domainanswersheet.NewOptionValue("A"),
		},
		{
			questionCode: "q2",
			questionType: questionnairedomain.TypeNumber,
			answerValue:  domainanswersheet.NewNumberValue(12),
		},
	})
	if err != nil {
		t.Fatalf("createAnswers returned error: %v", err)
	}
	if len(answers) != 2 {
		t.Fatalf("len(answers) = %d, want 2", len(answers))
	}
	if answers[0].QuestionCode() != "q1" || answers[1].QuestionCode() != "q2" {
		t.Fatalf("unexpected answers: %+v", answers)
	}
}
