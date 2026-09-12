package answersheet

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	start "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"time"
)

type AnsweringStartResolver interface {
	ResolveSubmission(context.Context, uint64, start.Intent) (sheet.StartContext, error)
}
type AnsweringStartResolverInjector interface{ SetAnsweringStartResolver(AnsweringStartResolver) }

func (s *submissionService) SetAnsweringStartResolver(resolver AnsweringStartResolver) {
	s.startResolver = resolver
}
func (s *submissionService) resolveStart(ctx context.Context, dto SubmitAnswerSheetDTO, admission sheet.Admission) (sheet.StartContext, error) {
	if dto.AnsweringStartID == 0 {
		return sheet.StartContext{}, nil
	}
	if s.startResolver == nil {
		return sheet.StartContext{}, errors.WithCode(code.ErrInternalServerError, "作答开始记录解析器不可用")
	}
	origin, err := originRefFromDTO(dto)
	if err != nil {
		return sheet.StartContext{}, err
	}
	value, err := s.startResolver.ResolveSubmission(ctx, dto.AnsweringStartID, start.Intent{
		OrgID: int64(dto.OrgID), UserID: dto.FillerID, TesteeID: dto.TesteeID,
		QuestionnaireCode: dto.QuestionnaireCode, QuestionnaireVersion: dto.QuestionnaireVer,
		ModelCode: admission.ModelCode(), ModelVersion: admission.ModelVersion(), Origin: origin,
	})
	if err != nil {
		return sheet.StartContext{}, errors.WrapC(err, code.ErrInvalidArgument, "作答开始记录与提交不匹配或不可用")
	}
	if value.IsZero() || value.ID() != dto.AnsweringStartID {
		return sheet.StartContext{}, errors.WithCode(code.ErrInternalServerError, "作答开始记录不完整")
	}
	return value, nil
}

// ValidateStartContent reuses Survey's published-content and source admission.
// Loading a questionnaire alone never creates an answering-start record.
func (s *submissionService) ValidateStartContent(ctx context.Context, i *start.Intent) error {
	if s.binding == nil || s.questionnaireRepo == nil || s.attribution == nil {
		return errors.WithCode(code.ErrInternalServerError, "作答内容准入不可用")
	}
	dto := SubmitAnswerSheetDTO{OrgID: uint64(i.OrgID), TesteeID: i.TesteeID, FillerID: i.UserID, QuestionnaireCode: i.QuestionnaireCode, QuestionnaireVer: i.QuestionnaireVersion, OriginRef: &OriginRefDTO{Type: string(i.Origin.Type), ID: i.Origin.ID}}
	if i.Origin.Type == sheet.OriginTypePlanTask {
		dto.TaskID = i.Origin.ID
	}
	if _, _, err := s.fetchAndValidateQuestionnaire(ctx, logger.L(ctx), &dto); err != nil {
		return err
	}
	admission, err := s.resolveAdmission(ctx, i.QuestionnaireCode, i.QuestionnaireVersion)
	if err != nil {
		return err
	}
	if (i.ModelCode != "" || i.ModelVersion != "") && (admission.ModelCode() != i.ModelCode || admission.ModelVersion() != i.ModelVersion) {
		return errors.WithCode(code.ErrInvalidArgument, "开始内容版本与当前准入不匹配")
	}
	// Freeze the server-resolved model binding; ordinary questionnaires need not
	// submit model metadata, while explicit model versions must match.
	i.ModelCode, i.ModelVersion = admission.ModelCode(), admission.ModelVersion()
	_, err = s.resolveAttribution(ctx, dto, admission, time.Now().UTC())
	return err
}
