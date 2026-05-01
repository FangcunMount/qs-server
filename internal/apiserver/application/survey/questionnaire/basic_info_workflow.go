package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// UpdateBasicInfo 更新基本信息
func (s *lifecycleService) UpdateBasicInfo(ctx context.Context, dto UpdateQuestionnaireBasicInfoDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("更新基本信息",
		"action", "update_basic_info",
		"code", dto.Code,
		"title", dto.Title,
		"type", dto.Type,
	)

	if err := s.validateBasicInfoInput(ctx, dto); err != nil {
		return nil, err
	}

	q, err := s.loadEditableHead(ctx, dto.Code, "update_basic_info", "编辑")
	if err != nil {
		return nil, err
	}

	l.Debugw("更新基本信息",
		"action", "update_basic_info",
		"code", dto.Code,
	)
	baseInfo := domainQuestionnaire.BaseInfo{}
	if err := baseInfo.UpdateAll(q, dto.Title, dto.Description, dto.ImgUrl, domainQuestionnaire.NormalizeQuestionnaireType(dto.Type)); err != nil {
		l.Errorw("更新基本信息失败",
			"action", "update_basic_info",
			"code", dto.Code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "更新基本信息失败")
	}

	if err := s.persistQuestionnaire(ctx, q, dto.Code, "update_basic_info", "基本信息"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "update_basic_info", dto.Code, startTime)

	return toQuestionnaireResult(q), nil
}

func (s *lifecycleService) validateBasicInfoInput(ctx context.Context, dto UpdateQuestionnaireBasicInfoDTO) error {
	if err := s.validateCode(ctx, dto.Code, "update_basic_info"); err != nil {
		return err
	}
	if dto.Title == "" {
		logger.L(ctx).Warnw("问卷标题为空",
			"action", "update_basic_info",
			"code", dto.Code,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷标题不能为空")
	}
	return nil
}
