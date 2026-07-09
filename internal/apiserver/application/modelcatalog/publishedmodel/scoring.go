package publishedmodel

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func buildScoring(model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if model.Kind != domain.KindScale {
		return nil, fmt.Errorf("model kind %s is not scale", model.Kind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("scale model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("scale model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmScaleDefault
	}
	record := recordFromModel(model, domain.KindScale, domain.SubKindEmpty, algorithm, domain.PayloadFormatAssessmentScaleV1, domain.DecisionKindScoreRange, encoded)
	if version := scaleVersionFromPayload(encoded); version != "" {
		record.Version = version
	}
	return record, nil
}

func scaleVersionFromPayload(payload []byte) string {
	var snapshot struct {
		ScaleVersion string
	}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return ""
	}
	return snapshot.ScaleVersion
}

// BuildAssessmentSnapshotFromScale materializes an execution snapshot from a
// legacy scale snapshot adapter. New publishing flows should use
// BuildAssessmentSnapshot with AssessmentModel.
func BuildAssessmentSnapshotFromScale(model *scalesnapshot.ScaleSnapshot) (*port.AssessmentSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("scale snapshot is nil")
	}
	payload, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("marshal scale payload: %w", err)
	}
	status := model.Status
	if status == "" {
		status = string(domain.ModelStatusPublished)
	}
	version := model.ScaleVersion
	if version == "" {
		version = model.QuestionnaireVersion
	}
	return &port.AssessmentSnapshot{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        domain.PayloadFormatAssessmentScaleV1,
		ProductChannel:       domain.ProductChannelMedicalScale,
		Kind:                 domain.KindScale,
		SubKind:              domain.SubKindEmpty,
		Algorithm:            domain.AlgorithmScaleDefault,
		Code:                 model.Code,
		Version:              version,
		Title:                model.Title,
		Status:               status,
		DecisionKind:         domain.DecisionKindScoreRange,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Source:               map[string]any{},
		Payload:              payload,
	}, nil
}

// BuildFromScale materializes a published runtime record from a legacy scale snapshot.
//
// Deprecated: use BuildAssessmentSnapshotFromScale.
func BuildFromScale(model *scalesnapshot.ScaleSnapshot) (*port.PublishedModel, error) {
	return BuildAssessmentSnapshotFromScale(model)
}
