package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) resolveQuestionnaireBinding() questionnaireBindingResolver {
	return questionnaireBindingResolver{
		repo:                 s.repo,
		questionnaireCatalog: s.questionnaireCatalog,
		baseInfo:             s.baseInfo,
	}
}

type questionnaireBindingResolver struct {
	repo                 domainScale.Repository
	questionnaireCatalog questionnairecatalog.Catalog
	baseInfo             domainScale.BaseInfo
}

func (r questionnaireBindingResolver) ensureQuestionnaireVersion(ctx context.Context, scaleCode string, m *domainScale.MedicalScale) error {
	if m.GetQuestionnaireCode().IsEmpty() {
		return nil
	}

	if err := r.validate(ctx, m.GetQuestionnaireCode().Value(), m.GetQuestionnaireVersion(), scaleCode); err != nil {
		return err
	}

	if m.GetQuestionnaireVersion() != "" {
		return nil
	}

	questionnaireCode := m.GetQuestionnaireCode().Value()
	logger.L(ctx).Infow("问卷版本为空，自动获取最新版本",
		"scale_code", scaleCode,
		"questionnaire_code", questionnaireCode,
	)

	if r.questionnaireCatalog == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}
	q, err := r.questionnaireCatalog.FindPublishedQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷失败")
	}
	if q == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}

	latestVersion := q.Version
	logger.L(ctx).Infow("自动设置问卷版本",
		"scale_code", scaleCode,
		"questionnaire_code", questionnaireCode,
		"version", latestVersion,
	)
	if err := r.baseInfo.UpdateQuestionnaire(m, m.GetQuestionnaireCode(), latestVersion); err != nil {
		return errors.WrapC(err, errorCode.ErrInvalidArgument, "更新问卷版本失败")
	}
	if err := r.repo.Update(ctx, m); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存问卷版本失败")
	}
	return nil
}

func (r questionnaireBindingResolver) validate(ctx context.Context, questionnaireCode string, questionnaireVersion string, currentScaleCode string) error {
	if questionnaireCode == "" {
		return nil
	}

	if r.questionnaireCatalog == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}
	q, err := r.questionnaireCatalog.FindQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷失败")
	}
	if q == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}
	if q.Type != "MedicalScale" {
		return errors.WithCode(errorCode.ErrInvalidArgument, "量表只能关联 MedicalScale 类型问卷")
	}

	if questionnaireVersion != "" {
		versioned, err := r.questionnaireCatalog.FindQuestionnaireVersion(ctx, questionnaireCode, questionnaireVersion)
		if err != nil {
			return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷版本失败")
		}
		if versioned == nil {
			return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷版本不存在")
		}
		if versioned.Type != "MedicalScale" {
			return errors.WithCode(errorCode.ErrInvalidArgument, "量表只能关联 MedicalScale 类型问卷")
		}
	}

	boundScale, err := r.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		if domainScale.IsNotFound(err) {
			return nil
		}
		return errors.WrapC(err, errorCode.ErrDatabase, "查询问卷关联量表失败")
	}
	if boundScale == nil {
		return nil
	}
	if currentScaleCode != "" && boundScale.GetCode().String() == currentScaleCode {
		return nil
	}
	return errors.WithCode(errorCode.ErrInvalidArgument, "该问卷已关联其他量表")
}
