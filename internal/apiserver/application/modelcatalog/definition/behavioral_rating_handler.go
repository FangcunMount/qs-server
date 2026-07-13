package definition

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

// BehavioralRatingDefinitionHandler 行为评定模型定义处理程序
type BehavioralRatingDefinitionHandler struct {
	NormRepo port.NormRepository
}

// Supports 支持
func (BehavioralRatingDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindBehavioralRating
}

// ValidateForPublish 验证发布
func (h BehavioralRatingDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{{Field: "model", Message: "模型不能为空", Code: "model.required", Level: domain.ValidationLevelError}}
	}
	if model.Definition.IsEmpty() {
		return []domain.DomainValidationIssue{{Field: "definition", Message: "行为评定模型定义不能为空", Code: "definition.required", Level: domain.ValidationLevelError}}
	}
	issues := model.ValidateForPublish().Issues
	issues = append(issues, ValidateDefinitionV2ForPublish(ctx, model.DefinitionV2, h.NormRepo)...)
	if model.Algorithm == domain.AlgorithmBrief2 && model.DefinitionV2 != nil && model.DefinitionV2.Execution.Brief2 == nil {
		issues = append(issues, domain.DomainValidationIssue{Field: "execution.brief2", Code: "brief2.execution.required", Message: "BRIEF-2 execution spec is required", Level: domain.ValidationLevelError})
	}
	if _, err := model.DecisionKindForDefinition(); err != nil {
		issues = append(issues, domain.DomainValidationIssue{Field: "definition_v2.conclusions", Code: "definition_v2.decision.invalid", Message: err.Error(), Level: domain.ValidationLevelError})
	}
	return issues
}

// BuildSnapshotPayload 构建快照负载
func (h BehavioralRatingDefinitionHandler) BuildSnapshotPayload(ctx context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	if model == nil || model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("behavioral_rating definition_v2 is required")
	}
	table, err := h.brief2NormTable(ctx, model.DefinitionV2)
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	encoded, err := behavioralpayload.PayloadFromDefinitionWithNorm(model.DefinitionV2, table)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("project behavioral_rating payload: %w", err)
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmBehavioralRatingDefault
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	return SnapshotBuildResult{Kind: domain.KindBehavioralRating, Algorithm: algorithm, PayloadFormat: domain.PayloadFormatForBehavioralRating(algorithm), DecisionKind: decisionKind, Payload: encoded}, nil
}

// brief2NormTable 构建BRIEF-2规范表
func (h BehavioralRatingDefinitionHandler) brief2NormTable(ctx context.Context, value *domain.Definition) (*domain.Norm, error) {
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
		return nil, nil
	}
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
