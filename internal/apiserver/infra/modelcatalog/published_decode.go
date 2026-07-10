package modelcatalog

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// DecodeBehavioralRatingFromPublished is the single infra entry for behavioral_rating snapshot decode.
func DecodeBehavioralRatingFromPublished(model *port.PublishedModel) (*behavioralsnapshot.Snapshot, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.Kind != domain.KindBehavioralRating {
		return nil, fmt.Errorf("published model kind = %q, want behavioral_rating", model.Kind)
	}
	if !domain.IsBehavioralRatingPayloadFormat(model.PayloadFormat) {
		return nil, fmt.Errorf("unsupported behavioral_rating payload format: %s", model.PayloadFormat)
	}
	payload, err := behavioralsnapshot.ParsePublishedPayload(
		model.PayloadFormat,
		model.Code,
		model.Version,
		model.Title,
		model.Status,
		model.Payload,
	)
	if err != nil {
		return nil, err
	}
	payload.QuestionnaireCode = model.QuestionnaireCode
	payload.QuestionnaireVersion = model.QuestionnaireVersion
	if !payload.IsPublished() {
		return nil, fmt.Errorf("behavioral_rating model is not published: %s", payload.Code)
	}
	return payload, nil
}

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
	applyScaleWireMetadata(snapshot, model.Payload)
	return snapshot, nil
}

// applyScaleWireMetadata reads only the legacy wire identifier. It is not part
// of DefinitionV2 semantics, but legacy binding adapters still need it.
func applyScaleWireMetadata(snapshot *scalesnapshot.ScaleSnapshot, payload []byte) {
	if snapshot == nil || len(payload) == 0 {
		return
	}
	var wire struct {
		ID uint64 `json:"id"`
	}
	if json.Unmarshal(payload, &wire) == nil && snapshot.ID == 0 {
		snapshot.ID = wire.ID
	}
}
