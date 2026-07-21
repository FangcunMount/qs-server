package definition

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralruntime "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	cognitiveruntime "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	scaleruntime "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	typologyruntime "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// RuntimeMaterializer proves that a canonical DefinitionV2 can be projected to
// its family runtime DTO without producing or persisting compatibility bytes.
type RuntimeMaterializer struct{}

func (RuntimeMaterializer) MaterializeScale(model *domain.AssessmentModel) (Materialization, error) {
	if model == nil || model.DefinitionV2 == nil {
		return Materialization{}, fmt.Errorf("scale definition_v2 is required")
	}
	snapshot := scaleruntime.ScaleSnapshotFromDefinition(scaleruntime.ExecutionEnvelope{
		Code: model.Code, ScaleVersion: modelRevisionVersion(model), Title: model.Title,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status: string(domain.ModelStatusPublished),
	}, model.DefinitionV2)
	if snapshot == nil {
		return Materialization{}, fmt.Errorf("materialize scale runtime: empty snapshot")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmScaleDefault
	}
	decision, err := model.DecisionKindForDefinition()
	if err != nil {
		return Materialization{}, err
	}
	return completeMaterialization(domain.KindScale, domain.SubKindEmpty, algorithm, decision, snapshot.ScaleVersion)
}

// MaterializeTypologyRuntime returns the temporary in-memory runtime DTO used
// by publish validation and report preview.
func (RuntimeMaterializer) MaterializeTypologyRuntime(model *domain.AssessmentModel, status string) (*typologyruntime.Payload, error) {
	if model == nil || model.DefinitionV2 == nil {
		return nil, fmt.Errorf("typology definition_v2 is required")
	}
	return typologyruntime.PayloadFromDefinition(typologyruntime.DefinitionEnvelope{
		Code: model.Code, Version: modelRevisionVersion(model), Title: model.Title,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status: status, Algorithm: model.Algorithm,
	}, model.DefinitionV2)
}

func (m RuntimeMaterializer) MaterializeTypology(model *domain.AssessmentModel) (Materialization, error) {
	if model == nil || model.DefinitionV2 == nil {
		return Materialization{}, fmt.Errorf("typology definition_v2 is required")
	}
	if model.SubKind != domain.SubKindTypology {
		return Materialization{}, fmt.Errorf("typology model sub_kind %s is not typology", model.SubKind)
	}
	if _, err := m.MaterializeTypologyRuntime(model, string(domain.ModelStatusPublished)); err != nil {
		return Materialization{}, err
	}
	decision, err := model.DecisionKindForDefinition()
	if err != nil {
		return Materialization{}, err
	}
	return completeMaterialization(domain.KindTypology, domain.SubKindTypology, model.Algorithm, decision, "")
}

func (RuntimeMaterializer) MaterializeCognitive(model *domain.AssessmentModel) (Materialization, error) {
	if model == nil || model.DefinitionV2 == nil {
		return Materialization{}, fmt.Errorf("cognitive definition_v2 is required")
	}
	if !domain.IsCanonicalPublishAlgorithm(model.Kind, model.Algorithm) {
		if domain.ClassifyAlgorithmWritePolicy(model.Kind, model.Algorithm) == domain.AlgorithmWriteDraftOK {
			return Materialization{}, fmt.Errorf("%s", publishAlgorithmRequiredMessage(model.Kind))
		}
		return Materialization{}, fmt.Errorf("algorithm %q is not supported for cognitive", model.Algorithm)
	}
	if _, err := cognitiveruntime.SnapshotFromDefinition(cognitiveruntime.DefinitionEnvelope{
		Code: model.Code, Version: modelRevisionVersion(model), Title: model.Title,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status: string(domain.ModelStatusPublished),
	}, model.DefinitionV2); err != nil {
		return Materialization{}, fmt.Errorf("materialize cognitive runtime: %w", err)
	}
	decision, err := model.DecisionKindForDefinition()
	if err != nil {
		return Materialization{}, err
	}
	return completeMaterialization(domain.KindCognitive, domain.SubKindEmpty, model.Algorithm, decision, "")
}

func (RuntimeMaterializer) MaterializeBehavioral(model *domain.AssessmentModel, table *domain.Norm) (Materialization, error) {
	if model == nil || model.DefinitionV2 == nil {
		return Materialization{}, fmt.Errorf("behavioral_rating definition_v2 is required")
	}
	switch domain.ClassifyAlgorithmWritePolicy(model.Kind, model.Algorithm) {
	case domain.AlgorithmWriteCanonical:
	case domain.AlgorithmWriteDraftOK:
		return Materialization{}, fmt.Errorf("%s", publishAlgorithmRequiredMessage(model.Kind))
	default:
		return Materialization{}, fmt.Errorf("algorithm %q is not supported for behavioral_rating", model.Algorithm)
	}
	tables := map[string]*domain.Norm{}
	if table != nil {
		tables[table.TableVersion] = table
	}
	if _, err := behavioralruntime.SnapshotFromDefinition(behavioralruntime.DefinitionEnvelope{
		Code: model.Code, Version: modelRevisionVersion(model), Title: model.Title,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status: string(domain.ModelStatusPublished),
	}, model.DefinitionV2, tables); err != nil {
		return Materialization{}, fmt.Errorf("materialize behavioral_rating runtime: %w", err)
	}
	decision, err := model.DecisionKindForDefinition()
	if err != nil {
		return Materialization{}, err
	}
	if decision != domain.DecisionKindNormLookup {
		return Materialization{}, fmt.Errorf("behavioral_rating decision kind must be norm_lookup, got %s", decision)
	}
	return completeMaterialization(domain.KindBehavioralRating, domain.SubKindEmpty, model.Algorithm, decision, "")
}

func completeMaterialization(kind domain.Kind, subKind domain.SubKind, algorithm domain.Algorithm, decision domain.DecisionKind, version string) (Materialization, error) {
	runtime, err := domain.ResolveRuntimeIdentity(kind, subKind, algorithm, decision)
	if err != nil {
		return Materialization{}, err
	}
	return Materialization{Kind: kind, SubKind: subKind, Algorithm: algorithm, AlgorithmFamily: runtime.AlgorithmFamily, DecisionKind: decision, Version: version}, nil
}

func modelRevisionVersion(model *domain.AssessmentModel) string {
	return fmt.Sprintf("v%d", model.Revision())
}
