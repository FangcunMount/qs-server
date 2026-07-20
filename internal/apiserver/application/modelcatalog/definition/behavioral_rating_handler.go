package definition

import (
	"context"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

// BehavioralRatingDefinitionHandler composes shared validators with behavioral
// Norm loading and payload projection.
type BehavioralRatingDefinitionHandler struct {
	NormRepo           port.NormRepository
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
}

// Supports 支持
func (BehavioralRatingDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindBehavioralRating
}

// ValidateForPublish 验证发布
func (h BehavioralRatingDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{modelRequiredIssue()}
	}
	if model.Definition.IsEmpty() {
		return []domain.DomainValidationIssue{{
			Field: "definition", Message: "行为评定模型定义不能为空",
			Code: "definition.required", Level: domain.ValidationLevelError,
		}}
	}
	issues := model.ValidateForPublish().Issues
	issues = append(issues, ValidateDefinitionForPublish(ctx, model, h.NormRepo)...)
	issues = append(issues, ValidateBehavioralSemantic(model)...)
	issues = append(issues, ValidateAlgorithmBinding(model)...)
	issues = AppendDecisionKindIssues(model, issues)
	issues = append(issues, ValidateQuestionnaireMeasure(ctx, h.QuestionnaireQuery, model)...)
	return issues
}

// BuildSnapshotPayload 构建快照负载
func (h BehavioralRatingDefinitionHandler) BuildSnapshotPayload(ctx context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	if model == nil || model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("behavioral_rating definition_v2 is required")
	}
	if err := requireBehavioralPublishAlgorithm(model.Algorithm); err != nil {
		return SnapshotBuildResult{}, err
	}
	table, err := h.loadNormTable(ctx, model.DefinitionV2)
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	encoded, err := behavioralpayload.PayloadFromDefinitionWithNorm(model.DefinitionV2, table)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("project behavioral_rating payload: %w", err)
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	if decisionKind != domain.DecisionKindNormLookup {
		return SnapshotBuildResult{}, fmt.Errorf("behavioral_rating decision kind must be norm_lookup, got %s", decisionKind)
	}
	return SnapshotBuildResult{
		Kind:          domain.KindBehavioralRating,
		Algorithm:     model.Algorithm,
		PayloadFormat: domain.PayloadFormatForBehavioralRating(model.Algorithm),
		DecisionKind:  decisionKind,
		Payload:       encoded,
	}, nil
}

func (h BehavioralRatingDefinitionHandler) loadNormTable(ctx context.Context, value *domain.Definition) (*domain.Norm, error) {
	if value == nil {
		return nil, nil
	}
	version := ""
	for _, ref := range value.Calibration.NormRefs {
		if ref.NormTableVersion == "" {
			continue
		}
		if version != "" && version != ref.NormTableVersion {
			return nil, fmt.Errorf("behavioral_rating definition references multiple norm table versions: %s and %s", version, ref.NormTableVersion)
		}
		version = ref.NormTableVersion
	}
	if version == "" {
		return nil, fmt.Errorf("behavioral_rating requires a Norm table version")
	}
	return h.loadNormTableByVersion(ctx, version)
}

func (h BehavioralRatingDefinitionHandler) loadNormTableByVersion(ctx context.Context, version string) (*domain.Norm, error) {
	if h.NormRepo == nil {
		return nil, fmt.Errorf("behavioral_rating norm repository is required")
	}
	table, err := h.NormRepo.FindNorm(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("load behavioral_rating norm table %s: %w", version, err)
	}
	if table == nil || table.TableVersion != version {
		return nil, fmt.Errorf("behavioral_rating norm table %s is not available", version)
	}
	return table, nil
}
