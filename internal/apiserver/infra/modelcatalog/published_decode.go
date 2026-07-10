package modelcatalog

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// DecodeBehavioralRatingFromDefinition builds a behavioral execution snapshot from DefinitionV2.
func DecodeBehavioralRatingFromDefinition(model *port.PublishedModel, tables map[string]*norm.Norm) (*behavioralsnapshot.Snapshot, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.Kind != domain.KindBehavioralRating {
		return nil, fmt.Errorf("published model kind = %q, want behavioral_rating", model.Kind)
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("behavioral_rating definition_v2 is required for runtime: %s", model.Code)
	}
	payload, err := behavioralsnapshot.SnapshotFromDefinition(behavioralsnapshot.DefinitionEnvelope{
		Code: model.Code, Version: model.Version, Title: model.Title, QuestionnaireCode: model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status,
	}, model.DefinitionV2, tables)
	if err != nil {
		return nil, err
	}
	if !payload.IsPublished() {
		return nil, fmt.Errorf("behavioral_rating model is not published: %s", payload.Code)
	}
	return payload, nil
}

// DecodeScaleFromPublished decodes a v2 published scale snapshot.
func DecodeScaleFromPublished(model *port.PublishedModel) (*scalesnapshot.ScaleSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("published model snapshot is nil")
	}
	if model.Kind != domain.KindScale {
		return nil, fmt.Errorf("published model kind = %q, want scale", model.Kind)
	}
	format := model.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatAssessmentScaleV1
	}
	if !domain.IsScalePayloadFormat(format) {
		return nil, fmt.Errorf("unsupported scale payload format: %s", format)
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("scale definition_v2 is required for runtime: %s", model.Code)
	}
	snapshot := scalesnapshot.ScaleSnapshotFromDefinition(scalesnapshot.ExecutionEnvelope{
		Code:                 model.Code,
		ScaleVersion:         model.Version,
		Title:                model.Title,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
	}, model.DefinitionV2)
	if snapshot == nil {
		return nil, fmt.Errorf("scale definition_v2 cannot produce runtime snapshot: %s", model.Code)
	}
	applyScaleWireMetadata(snapshot, model)
	return snapshot, nil
}

// applyScaleWireMetadata reads only the legacy wire identifier. It is not part
// of DefinitionV2 semantics, but legacy binding adapters still need it.
func applyScaleWireMetadata(snapshot *scalesnapshot.ScaleSnapshot, model *port.PublishedModel) {
	if snapshot == nil || model == nil {
		return
	}
	legacy, ok := port.LegacyScaleBindingFromPublished(model)
	if !ok {
		return
	}
	if snapshot.ID == 0 {
		snapshot.ID = legacy.MedicalScaleID
	}
	if legacy.ScaleVersion != "" {
		snapshot.ScaleVersion = legacy.ScaleVersion
	}
}
