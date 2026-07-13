package binding

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// TypologyPolicy 保持类型合同，即绑定的问卷必须已经发布并包含问题
type TypologyPolicy struct {
	Questionnaires questionnaireapp.QuestionnaireQueryService
}

// Supports 支持
func (p TypologyPolicy) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindTypology
}

// Validate 验证
func (p TypologyPolicy) Validate(ctx context.Context, _ *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	if binding.QuestionnaireCode == "" {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "questionnaire code is required")
	}
	if p.Questionnaires == nil {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInternalServerError, "questionnaire query service is not configured")
	}
	var (
		result *questionnaireapp.QuestionnaireResult
		err    error
	)
	if binding.QuestionnaireVersion != "" {
		result, err = p.Questionnaires.GetPublishedByCodeVersion(ctx, binding.QuestionnaireCode, binding.QuestionnaireVersion)
	} else {
		result, err = p.Questionnaires.GetPublishedByCode(ctx, binding.QuestionnaireCode)
	}
	if err != nil || result == nil || result.Version == "" {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "binding questionnaire is invalid")
	}
	if len(result.Questions) == 0 {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "binding questionnaire must contain questions")
	}
	return domain.QuestionnaireBinding{QuestionnaireCode: result.Code, QuestionnaireVersion: result.Version}, nil
}

// BeforePublish 发布前
func (TypologyPolicy) BeforePublish(context.Context, *domain.AssessmentModel) error {
	return nil
}

// ScalePolicy 保持量表特定的问卷类型、唯一性和发布版本同步规则
type ScalePolicy struct {
	Models               modelcatalogport.ModelRepository
	Questionnaires       questionnairecatalog.Catalog
	PublishQuestionnaire func(context.Context, string) (string, error)
}

// Supports 支持
func (p ScalePolicy) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindScale
}

// Validate 验证
func (p ScalePolicy) Validate(ctx context.Context, model *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	if binding.QuestionnaireCode == "" {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "questionnaire code is required")
	}
	if p.Questionnaires == nil || p.Models == nil {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInternalServerError, "scale questionnaire binding dependencies are not configured")
	}
	head, err := p.Questionnaires.FindQuestionnaire(ctx, binding.QuestionnaireCode)
	if err != nil || head == nil {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrQuestionnaireNotFound, "bound questionnaire does not exist")
	}
	if head.Type != "MedicalScale" {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "scale can only bind a MedicalScale questionnaire")
	}
	if binding.QuestionnaireVersion != "" {
		versioned, err := p.Questionnaires.FindQuestionnaireVersion(ctx, binding.QuestionnaireCode, binding.QuestionnaireVersion)
		if err != nil || versioned == nil {
			return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrQuestionnaireNotFound, "bound questionnaire version does not exist")
		}
		if versioned.Type != "MedicalScale" {
			return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "scale can only bind a MedicalScale questionnaire")
		}
	}
	bound, err := p.Models.FindByQuestionnaireCode(ctx, domain.KindScale, binding.QuestionnaireCode)
	if err != nil && !domain.IsNotFound(err) {
		return domain.QuestionnaireBinding{}, err
	}
	if bound != nil && (model == nil || bound.Code != model.Code) {
		return domain.QuestionnaireBinding{}, errors.WithCode(code.ErrInvalidArgument, "questionnaire is already bound to another scale")
	}
	return binding, nil
}

// BeforePublish 发布前
func (p ScalePolicy) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	if model == nil || model.Binding.QuestionnaireCode == "" {
		return nil
	}
	if _, err := p.Validate(ctx, model, model.Binding); err != nil {
		return err
	}
	if p.Questionnaires == nil {
		return errors.WithCode(code.ErrQuestionnaireNotFound, "bound questionnaire does not exist")
	}
	head, err := p.Questionnaires.FindQuestionnaire(ctx, model.Binding.QuestionnaireCode)
	if err != nil || head == nil {
		return errors.WithCode(code.ErrQuestionnaireNotFound, "bound questionnaire does not exist")
	}
	published, err := p.Questionnaires.FindPublishedQuestionnaire(ctx, model.Binding.QuestionnaireCode)
	if err != nil {
		return errors.WithCode(code.ErrQuestionnaireNotFound, "published bound questionnaire does not exist")
	}
	targetVersion := model.Binding.QuestionnaireVersion
	publishedVersion := ""
	if published != nil {
		publishedVersion = published.Version
	}
	if shouldPublishBoundQuestionnaire(head.Status, head.Version, publishedVersion) {
		if p.PublishQuestionnaire == nil {
			return errors.WithCode(code.ErrInternalServerError, "scale publication requires questionnaire publisher")
		}
		version, err := p.PublishQuestionnaire(ctx, model.Binding.QuestionnaireCode)
		if err != nil {
			return errors.WrapC(err, code.ErrInvalidArgument, "publish bound questionnaire")
		}
		targetVersion = version
	} else if publishedVersion != "" {
		targetVersion = publishedVersion
	} else if targetVersion == "" {
		targetVersion = head.Version
	}
	if targetVersion == "" {
		return errors.WithCode(code.ErrQuestionnaireNotFound, "bound questionnaire version does not exist")
	}
	if targetVersion == model.Binding.QuestionnaireVersion {
		return nil
	}
	return model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: targetVersion}, time.Now().UTC())
}

func shouldPublishBoundQuestionnaire(headStatus, headVersion, activePublishedVersion string) bool {
	if headStatus == "draft" {
		return true
	}
	if activePublishedVersion == "" && headStatus != "published" {
		return true
	}
	return headVersion != "" && activePublishedVersion != "" && headVersion != activePublishedVersion && headStatus != "published"
}
