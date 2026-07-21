package definition

import (
	"context"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// BehavioralRatingDefinitionHandler composes shared validators with behavioral
// Norm loading and payload projection.
type BehavioralRatingDefinitionHandler struct {
	NormRepo           port.NormRepository
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
}

// Supports 支持
func (BehavioralRatingDefinitionHandler) Supports(identity domain.Identity) bool {
	return supportsBinding(domain.KindBehavioralRating, identity)
}

// ValidateForPublish 验证发布
func (h BehavioralRatingDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	return ComposePublishValidation(ctx, model, PublicationComposerOptions{
		NormRepo:                  h.NormRepo,
		QuestionnaireQuery:        h.QuestionnaireQuery,
		IncludeBehavioralSemantic: true,
		IncludeAlgorithmBinding:   true,
		StrategyCapabilityPath:    capability.PathBehavioralRatingDescriptor,
	})
}

// MaterializeSnapshot validates the DefinitionV2 behavioral runtime projection.
func (h BehavioralRatingDefinitionHandler) MaterializeSnapshot(ctx context.Context, model *domain.AssessmentModel) (Materialization, error) {
	if model == nil || model.DefinitionV2 == nil {
		return Materialization{}, fmt.Errorf("behavioral_rating definition_v2 is required")
	}
	table, err := h.loadNormTable(ctx, model.DefinitionV2)
	if err != nil {
		return Materialization{}, err
	}
	return (RuntimeMaterializer{}).MaterializeBehavioral(model, table)
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
