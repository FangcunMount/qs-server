package modelcatalog

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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
	if model.DefinitionV2 != nil {
		if snapshot := scalesnapshot.ScaleSnapshotFromDefinition(scalesnapshot.ExecutionEnvelope{
			Code:                 model.Code,
			ScaleVersion:         model.Version,
			Title:                model.Title,
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
			Status:               model.Status,
		}, model.DefinitionV2); snapshot != nil {
			applyScalePublishedPayloadMetadata(snapshot, model.Payload)
			return snapshot, nil
		}
	}
	return scalesnapshot.ParsePublishedPayload(model.Payload)
}

func applyScalePublishedPayloadMetadata(snapshot *scalesnapshot.ScaleSnapshot, payload []byte) {
	if snapshot == nil || len(payload) == 0 {
		return
	}
	legacy, err := scalesnapshot.ParsePublishedPayload(payload)
	if err != nil || legacy == nil {
		return
	}
	if snapshot.ID == 0 {
		snapshot.ID = legacy.ID
	}
}
