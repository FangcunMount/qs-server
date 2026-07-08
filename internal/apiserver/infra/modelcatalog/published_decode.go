package modelcatalog

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

// DecodeBehavioralRatingFromPublished is the single infra entry for behavioral_rating snapshot decode.
func DecodeBehavioralRatingFromPublished(snapshot *domain.PublishedModelSnapshot) (*behavioralsnapshot.Snapshot, error) {
	if snapshot == nil {
		return nil, domain.ErrNotFound
	}
	if snapshot.Model.Kind != domain.KindBehavioralRating {
		return nil, fmt.Errorf("published model kind = %q, want behavioral_rating", snapshot.Model.Kind)
	}
	if !domain.IsBehavioralRatingPayloadFormat(snapshot.PayloadFormat) {
		return nil, fmt.Errorf("unsupported behavioral_rating payload format: %s", snapshot.PayloadFormat)
	}
	payload, err := behavioralsnapshot.ParsePublishedPayload(
		snapshot.PayloadFormat,
		snapshot.Model.Code,
		snapshot.Model.Version,
		snapshot.Model.Title,
		snapshot.Model.Status,
		snapshot.Payload,
	)
	if err != nil {
		return nil, err
	}
	payload.QuestionnaireCode = snapshot.Binding.QuestionnaireCode
	payload.QuestionnaireVersion = snapshot.Binding.QuestionnaireVersion
	if !payload.IsPublished() {
		return nil, fmt.Errorf("behavioral_rating model is not published: %s", payload.Code)
	}
	return payload, nil
}

// DecodeScaleFromPublished decodes a v2 published scale snapshot.
func DecodeScaleFromPublished(snapshot *domain.PublishedModelSnapshot) (*scalesnapshot.ScaleSnapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("published model snapshot is nil")
	}
	if snapshot.Model.Kind != domain.KindScale {
		return nil, fmt.Errorf("published model kind = %q, want scale", snapshot.Model.Kind)
	}
	format := snapshot.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatAssessmentScaleV1
	}
	if !domain.IsScalePayloadFormat(format) {
		return nil, fmt.Errorf("unsupported scale payload format: %s", format)
	}
	return scalesnapshot.ParsePublishedPayload(snapshot.Payload)
}
