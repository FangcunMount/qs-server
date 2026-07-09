package lifecycle

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (s *lifecycleService) resolveQuestionnaireBinding() questionnaireBindingResolver {
	return questionnaireBindingResolver{
		modelRepo:            s.modelRepo,
		questionnaireCatalog: s.questionnaireCatalog,
	}
}

type questionnaireBindingResolver struct {
	modelRepo            modelBindingLookup
	questionnaireCatalog questionnairecatalog.Catalog
}

type modelBindingLookup interface {
	FindByQuestionnaireCode(ctx context.Context, kind domain.Kind, questionnaireCode string) (*domain.AssessmentModel, error)
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

	boundModel, err := r.modelRepo.FindByQuestionnaireCode(ctx, domain.KindScale, questionnaireCode)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil
		}
		return errors.WrapC(err, errorCode.ErrDatabase, "查询问卷关联量表失败")
	}
	if boundModel == nil {
		return nil
	}
	if currentScaleCode != "" && boundModel.Code == currentScaleCode {
		return nil
	}
	return errors.WithCode(errorCode.ErrInvalidArgument, "该问卷已关联其他量表")
}
