package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func (s *lifecycleService) createQuestionnaire(
	ctx context.Context,
	l *logger.RequestLogger,
	dto CreateQuestionnaireDTO,
) (*domainQuestionnaire.Questionnaire, error) {
	code, err := resolveQuestionnaireCreateCode(l, dto)
	if err != nil {
		return nil, err
	}

	version := domainQuestionnaire.NewVersion("1.0")
	if dto.Version != "" {
		version = domainQuestionnaire.NewVersion(dto.Version)
	}
	qType := domainQuestionnaire.NormalizeQuestionnaireType(dto.Type)

	l.Debugw("创建问卷领域模型",
		"action", "create",
		"code", code.String(),
		"version", version.String(),
		"type", qType.String(),
	)
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode(code.String()),
		dto.Title,
		domainQuestionnaire.WithDesc(dto.Description),
		domainQuestionnaire.WithImgUrl(dto.ImgUrl),
		domainQuestionnaire.WithVersion(version),
		domainQuestionnaire.WithStatus(domainQuestionnaire.STATUS_DRAFT),
		domainQuestionnaire.WithType(qType),
	)
	if err != nil {
		l.Errorw("创建问卷领域模型失败",
			"action", "create",
			"code", code.String(),
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "创建问卷失败")
	}

	l.Debugw("保存问卷到数据库",
		"action", "create",
		"code", code.String(),
	)
	if err := s.repo.Create(ctx, q); err != nil {
		l.Errorw("保存问卷失败",
			"action", "create",
			"code", code.String(),
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	return q, nil
}

func resolveQuestionnaireCreateCode(l *logger.RequestLogger, dto CreateQuestionnaireDTO) (meta.Code, error) {
	if dto.Code != "" {
		l.Debugw("使用外部提供的问卷编码",
			"action", "create",
			"code", dto.Code,
		)
		return meta.NewCode(dto.Code), nil
	}

	code, err := meta.GenerateCode()
	if err != nil {
		l.Errorw("生成问卷编码失败",
			"action", "create",
			"result", "failed",
			"error", err.Error(),
		)
		return meta.NewCode(""), errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "生成问卷编码失败")
	}
	l.Debugw("生成新的问卷编码",
		"action", "create",
		"code", code.String(),
	)
	return code, nil
}
