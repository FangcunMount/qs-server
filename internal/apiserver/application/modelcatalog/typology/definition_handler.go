package typology

import (
	"context"
	"encoding/json"
	"fmt"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

// DefinitionHandler owns typology-specific definition validation and publish shaping.
type DefinitionHandler struct {
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
}

func (h DefinitionHandler) Supports(identity domain.Identity) bool {
	return domain.IsTypologyKind(identity.Kind)
}

func (h DefinitionHandler) PrepareForSave(_ context.Context, model *domain.AssessmentModel, input appdefinition.SaveInput) (appdefinition.SaveResult, []domain.DomainValidationIssue, error) {
	format := input.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	if issues := validateDefinitionPayloadForSave(format, input.Payload); len(issues) > 0 {
		return appdefinition.SaveResult{}, validationIssuesToDomain(issues), nil
	}
	algorithm := domain.Algorithm(input.Algorithm)
	if model != nil && model.Algorithm != "" {
		algorithm = model.Algorithm
	}
	storedPayload, err := normalizeDefinitionPayloadForStorage(input.Payload, algorithm)
	if err != nil {
		return appdefinition.SaveResult{}, nil, err
	}
	result := appdefinition.SaveResult{
		Payload: domain.DefinitionPayload{
			Format: format,
			Data:   storedPayload,
		},
	}
	if definitionV2, err := modeltypology.DefinitionFromPayload(storedPayload, algorithm); err == nil {
		result.DefinitionV2 = definitionV2
	}
	if input.Algorithm != "" {
		result.Algorithm = domain.Algorithm(input.Algorithm)
	}
	if input.SubKind != "" {
		result.SubKind = domain.SubKind(input.SubKind)
	}
	return result, nil, nil
}

func (h DefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	domainIssues := model.ValidateForPublish().Issues
	runtime, validationContext, definitionIssues := validateDefinitionPayloadForPublish(model)
	questionnaire, questionnaireIssues := questionnaireSnapshotForPublish(ctx, h.QuestionnaireQuery, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if len(definitionIssues) > 0 || len(questionnaireIssues) > 0 || runtime == nil {
		return mergeDomainValidationIssues(domainIssues, validationIssuesToDomain(definitionIssues), validationIssuesToDomain(questionnaireIssues))
	}
	runtimeIssues := modeltypology.ValidateRuntimeSpecForPublishWithContext(runtime, questionnaire, validationContext)
	return mergeDomainValidationIssues(domainIssues, runtimeIssues)
}

func (h DefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (appdefinition.SnapshotBuildResult, error) {
	if !domain.IsTypologyKind(model.Kind) {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("model kind %s is not typology", model.Kind)
	}
	if model.SubKind != domain.SubKindTypology {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("typology model sub_kind %s is not typology", model.SubKind)
	}
	if model.Definition.IsEmpty() {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("typology model definition is empty")
	}
	payload, runtime, err := modeltypology.PayloadAndRuntimeSpecFromDefinition(model.Definition.Data, model.Algorithm)
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, err
	}
	prepareTypologySnapshotPayload(payload, model, runtime)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("marshal typology payload: %w", err)
	}
	decisionKind := runtime.Decision.Kind
	if decisionKind == "" {
		return appdefinition.SnapshotBuildResult{}, fmt.Errorf("runtime decision.kind is required for publish")
	}
	return appdefinition.SnapshotBuildResult{
		Kind:          domain.KindTypology,
		SubKind:       domain.SubKindTypology,
		Algorithm:     model.Algorithm,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		DecisionKind:  decisionKind,
		Payload:       encoded,
	}, nil
}

func prepareTypologySnapshotPayload(payload *modeltypology.Payload, model *domain.AssessmentModel, runtime *modeltypology.RuntimeSpec) {
	payload.Code = model.Code
	payload.Version = "v" + fmt.Sprint(model.Revision())
	payload.Title = model.Title
	payload.QuestionnaireCode = model.Binding.QuestionnaireCode
	payload.QuestionnaireVersion = model.Binding.QuestionnaireVersion
	payload.Status = string(domain.ModelStatusPublished)
	payload.Algorithm = model.Algorithm
	payload.Runtime = runtime
}

func questionnaireSnapshotForPublish(ctx context.Context, query questionnaireapp.QuestionnaireQueryService, questionnaireCode, questionnaireVersion string) (modeltypology.QuestionnaireSnapshot, []ValidationIssue) {
	if questionnaireCode == "" || questionnaireVersion == "" {
		return modeltypology.QuestionnaireSnapshot{}, nil
	}
	if query == nil {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "问卷查询服务未配置",
			Code: "binding.questionnaire_query.unavailable", Level: "error",
		}}
	}
	q, err := query.GetPublishedByCodeVersion(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷不存在或未发布",
			Code: "binding.questionnaire.not_found", Level: "error",
		}}
	}
	if q == nil {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷不存在或未发布",
			Code: "binding.questionnaire.not_found", Level: "error",
		}}
	}
	if len(q.Questions) == 0 {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷题目不能为空",
			Code: "binding.questionnaire.questions.required", Level: "error",
		}}
	}
	return questionnaireSnapshotFromResult(q), nil
}

func validationIssuesToDomain(issues []ValidationIssue) []domain.DomainValidationIssue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]domain.DomainValidationIssue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, domain.DomainValidationIssue{
			Field:   issue.Field,
			Message: issue.Message,
			Code:    issue.Code,
			Level:   domain.ValidationLevel(issue.Level),
		})
	}
	return out
}

func mergeDomainValidationIssues(groups ...[]domain.DomainValidationIssue) []domain.DomainValidationIssue {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total == 0 {
		return nil
	}
	out := make([]domain.DomainValidationIssue, 0, total)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}
