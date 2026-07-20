package definition

import (
	"context"
	"encoding/json"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
)

// TypologyDefinitionHandler composes shared validators with typology payload
// projection. Report preview is owned by TypologyPreviewService.
type TypologyDefinitionHandler struct {
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
	ReportPreviewer    modelpreview.ReportPreviewer
}

// Supports 支持特定评估模型身份
func (TypologyDefinitionHandler) Supports(identity domain.Identity) bool {
	return supportsBinding(domain.KindTypology, identity)
}

// ValidateForPublish 验证发布
func (h TypologyDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	return ComposePublishValidation(ctx, model, PublicationComposerOptions{
		QuestionnaireQuery:          h.QuestionnaireQuery,
		StrategyCapabilityPath:      capability.PathTypologyDescriptor,
		SkipQuestionnaireOnDefError: true,
		OmitSharedTail:              true,
		AfterDefinition:             h.validateTypologyRuntime,
	})
}

func (h TypologyDefinitionHandler) validateTypologyRuntime(
	ctx context.Context,
	model *domain.AssessmentModel,
	issues []domain.DomainValidationIssue,
) []domain.DomainValidationIssue {
	payload, err := (CompatibilityPayloadProjector{}).ProjectTypologyPayload(model, "")
	if err != nil || payload == nil || payload.Runtime == nil {
		if err == nil {
			err = fmt.Errorf("typology runtime specification is empty")
		}
		return append(issues, domain.DomainValidationIssue{
			Field: "definition_v2", Code: "definition_v2.runtime.invalid",
			Message: err.Error(), Level: domain.ValidationLevelError,
		})
	}
	questionnaire, questionnaireIssues := loadPublishedQuestionnaire(ctx, h.QuestionnaireQuery, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if len(questionnaireIssues) > 0 {
		return append(issues, questionnaireIssues...)
	}
	return append(issues, modeltypology.ValidateRuntimeSpecForPublishWithContext(
		payload.Runtime,
		questionnaireSnapshotFromResult(questionnaire),
		modeltypology.RuntimeSpecValidationContext{Algorithm: payload.Algorithm, Outcomes: payload.Outcomes},
	)...)
}

// BuildSnapshotPayload 构建评估模型快照负载
func (TypologyDefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	return (CompatibilityPayloadProjector{}).ProjectTypology(model)
}

// PreviewReport 预览报告
func (h TypologyDefinitionHandler) PreviewReport(ctx context.Context, model *domain.AssessmentModel, raw json.RawMessage) (*PreviewResult, error) {
	return TypologyPreviewService{
		QuestionnaireQuery: h.QuestionnaireQuery,
		ReportPreviewer:    h.ReportPreviewer,
		ValidateForPublish: h.ValidateForPublish,
	}.PreviewReport(ctx, model, raw)
}

func questionnaireSnapshotFromResult(questionnaire *questionnaireapp.QuestionnaireResult) modeltypology.QuestionnaireSnapshot {
	if questionnaire == nil {
		return modeltypology.QuestionnaireSnapshot{}
	}
	snapshot := modeltypology.QuestionnaireSnapshot{Code: questionnaire.Code, Version: questionnaire.Version, Questions: make([]modeltypology.QuestionSnapshot, 0, len(questionnaire.Questions))}
	for _, question := range questionnaire.Questions {
		item := modeltypology.QuestionSnapshot{Code: question.Code, Type: question.Type, OptionCodes: make([]string, 0, len(question.Options))}
		for _, option := range question.Options {
			item.OptionCodes = append(item.OptionCodes, option.Value)
		}
		snapshot.Questions = append(snapshot.Questions, item)
	}
	return snapshot
}
