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
	issues = append(issues, h.validateBehavioralPublishContract(ctx, model)...)
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

func (h BehavioralRatingDefinitionHandler) validateBehavioralPublishContract(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	issues := make([]domain.DomainValidationIssue, 0)
	if err := requireBehavioralPublishAlgorithm(model.Algorithm); err != nil {
		issues = append(issues, domain.DomainValidationIssue{
			Field: "algorithm", Code: "behavioral_rating.algorithm.required", Message: err.Error(), Level: domain.ValidationLevelError,
		})
	}
	if model.DefinitionV2 == nil {
		return issues
	}
	def := model.DefinitionV2
	if len(def.Calibration.NormRefs) == 0 {
		issues = append(issues, domain.DomainValidationIssue{
			Field: "calibration.norm_refs", Code: "behavioral_rating.norm_refs.required",
			Message: "behavioral_rating 必须配置至少一条 NormRef；原始分区间模型应使用 scale", Level: domain.ValidationLevelError,
		})
	}

	normFactorsByVersion := map[string]map[string]struct{}{}
	for _, ref := range def.Calibration.NormRefs {
		if ref.NormTableVersion == "" {
			continue
		}
		factorSet, ok := normFactorsByVersion[ref.NormTableVersion]
		if !ok {
			table, err := h.loadNormTableByVersion(ctx, ref.NormTableVersion)
			if err != nil {
				// FindNorm failures are already reported by ValidateDefinitionV2ForPublish.
				continue
			}
			factorSet = normTableFactorSet(table)
			normFactorsByVersion[ref.NormTableVersion] = factorSet
		}
		if _, exists := factorSet[ref.FactorCode]; ref.FactorCode != "" && !exists {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "calibration.norm_refs", Code: "behavioral_rating.norm_ref.factor.missing_in_table",
				Message: fmt.Sprintf("NormRef factor %s 不在常模表 %s 中", ref.FactorCode, ref.NormTableVersion),
				Level:   domain.ValidationLevelError,
			})
		}
	}

	normRefFactors := map[string]struct{}{}
	for _, ref := range def.Calibration.NormRefs {
		if ref.FactorCode != "" {
			normRefFactors[ref.FactorCode] = struct{}{}
		}
	}
	for _, item := range def.Conclusions {
		normConclusion, ok := item.(domain.NormConclusion)
		if !ok {
			continue
		}
		if _, ok := normRefFactors[normConclusion.FactorCode]; normConclusion.FactorCode != "" && !ok {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "definition_v2.conclusions", Code: "behavioral_rating.conclusion.norm_ref.missing",
				Message: fmt.Sprintf("NormConclusion factor %s 缺少对应 NormRef", normConclusion.FactorCode),
				Level:   domain.ValidationLevelError,
			})
		}
	}
	return issues
}

func requireBehavioralPublishAlgorithm(algorithm domain.Algorithm) error {
	switch algorithm {
	case domain.AlgorithmBrief2, domain.AlgorithmSPMSensory:
		return nil
	case "":
		return fmt.Errorf("behavioral_rating 发布必须指定真实 Algorithm（brief2 或 spm_sensory）")
	case domain.AlgorithmBehavioralRatingDefault:
		return fmt.Errorf("新发布不允许 behavioral_rating_default；请使用 brief2 或 spm_sensory")
	default:
		return fmt.Errorf("behavioral_rating algorithm %q 不受支持；请使用 brief2 或 spm_sensory", algorithm)
	}
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

func normTableFactorSet(table *domain.Norm) map[string]struct{} {
	out := make(map[string]struct{})
	if table == nil {
		return out
	}
	for _, factor := range table.Factors {
		if factor.FactorCode != "" {
			out[factor.FactorCode] = struct{}{}
		}
	}
	return out
}
