package definition

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// CompatibilityPayloadProjector projects DefinitionV2 into family-specific
// published snapshot payloads. It has no publish-validation or preview side effects.
type CompatibilityPayloadProjector struct{}

// ProjectScale builds the scale snapshot payload for publication compatibility.
func (CompatibilityPayloadProjector) ProjectScale(model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	if model == nil {
		return SnapshotBuildResult{}, fmt.Errorf("scale assessment model is nil")
	}
	if model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("scale definition_v2 is required")
	}
	snapshot := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{
		Code:                 model.Code,
		ScaleVersion:         modelRevisionVersion(model),
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               "published",
	}, model.DefinitionV2)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("marshal scale snapshot: %w", err)
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmScaleDefault
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	return SnapshotBuildResult{
		Kind:          domain.KindScale,
		SubKind:       domain.SubKindEmpty,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatAssessmentScaleV1,
		DecisionKind:  decisionKind,
		Payload:       encoded,
		Version:       snapshot.ScaleVersion,
	}, nil
}

// ProjectTypologyPayload projects typology DefinitionV2 into the typed payload
// used by both publish validation and snapshot encoding.
func (CompatibilityPayloadProjector) ProjectTypologyPayload(model *domain.AssessmentModel, status string) (*modeltypology.Payload, error) {
	if model == nil || model.DefinitionV2 == nil {
		return nil, fmt.Errorf("typology definition_v2 is required")
	}
	return modeltypology.PayloadFromDefinition(modeltypology.DefinitionEnvelope{
		Code:                 model.Code,
		Version:              modelRevisionVersion(model),
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               status,
		Algorithm:            model.Algorithm,
	}, model.DefinitionV2)
}

// ProjectTypology builds the typology snapshot payload for publication compatibility.
func (p CompatibilityPayloadProjector) ProjectTypology(model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	if model == nil || model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("typology definition_v2 is required")
	}
	if model.SubKind != domain.SubKindTypology {
		return SnapshotBuildResult{}, fmt.Errorf("typology model sub_kind %s is not typology", model.SubKind)
	}
	payload, err := p.ProjectTypologyPayload(model, string(domain.ModelStatusPublished))
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("marshal typology payload: %w", err)
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	return SnapshotBuildResult{
		Kind:          domain.KindTypology,
		SubKind:       domain.SubKindTypology,
		Algorithm:     model.Algorithm,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		DecisionKind:  decisionKind,
		Payload:       encoded,
	}, nil
}

// ProjectCognitive builds the cognitive snapshot payload for publication compatibility.
func (CompatibilityPayloadProjector) ProjectCognitive(model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	if model == nil || model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("cognitive definition_v2 is required")
	}
	encoded, err := cognitivepayload.PayloadFromDefinition(model.DefinitionV2)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("project cognitive payload: %w", err)
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	return SnapshotBuildResult{
		Kind:          domain.KindCognitive,
		Algorithm:     algorithm,
		PayloadFormat: domain.PayloadFormatForCognitive(algorithm),
		DecisionKind:  decisionKind,
		Payload:       encoded,
	}, nil
}

// ProjectBehavioral builds the behavioral_rating snapshot payload given a loaded Norm table.
func (CompatibilityPayloadProjector) ProjectBehavioral(model *domain.AssessmentModel, table *domain.Norm) (SnapshotBuildResult, error) {
	if model == nil || model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("behavioral_rating definition_v2 is required")
	}
	if err := requireBehavioralPublishAlgorithm(model.Algorithm); err != nil {
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

func modelRevisionVersion(model *domain.AssessmentModel) string {
	return fmt.Sprintf("v%d", model.Revision())
}
