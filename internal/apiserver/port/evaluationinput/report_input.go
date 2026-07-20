package evaluationinput

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

const (
	// ReportInputSchemaLegacy is the historical shape: raw ModelPayload JSON only.
	ReportInputSchemaLegacy = 0
	// ReportInputSchemaV2 wraps payload with a frozen InterpretationAssets snapshot (MC-R016).
	ReportInputSchemaV2 = 2
)

type reportInputEnvelope struct {
	SchemaVersion        uint                          `json:"schema_version"`
	Payload              json.RawMessage               `json:"payload"`
	InterpretationAssets *interpretationassets.Assets  `json:"InterpretationAssets,omitempty"`
}

// MarshalReportInput freezes evaluation report input. When interpretation assets
// are available, schema v2 is used; otherwise the legacy payload-only shape is kept.
func MarshalReportInput(payload ModelPayload, assets *interpretationassets.Assets) ([]byte, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if assets == nil || !assets.IsMaterialized() {
		return rawPayload, nil
	}
	return json.Marshal(reportInputEnvelope{
		SchemaVersion:        ReportInputSchemaV2,
		Payload:              rawPayload,
		InterpretationAssets: assets,
	})
}

type decodedReportInput struct {
	Payload              ModelPayload
	InterpretationAssets *interpretationassets.Assets
}

func decodeReportInputBytes(data []byte, kind string) (decodedReportInput, error) {
	if len(data) == 0 {
		return decodedReportInput{}, nil
	}
	var peek struct {
		SchemaVersion *uint `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return decodedReportInput{}, err
	}
	if peek.SchemaVersion != nil && *peek.SchemaVersion >= ReportInputSchemaV2 {
		var envelope reportInputEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			return decodedReportInput{}, err
		}
		payload, err := decodeModelPayloadBytes(envelope.Payload, kind)
		if err != nil {
			return decodedReportInput{}, err
		}
		return decodedReportInput{Payload: payload, InterpretationAssets: envelope.InterpretationAssets}, nil
	}
	payload, err := decodeModelPayloadBytes(data, kind)
	if err != nil {
		return decodedReportInput{}, err
	}
	return decodedReportInput{Payload: payload}, nil
}

func decodeModelPayloadBytes(data []byte, kind string) (ModelPayload, error) {
	switch EvaluationModelKind(kind) {
	case EvaluationModelKindScale:
		var typed ScaleModelPayload
		if err := json.Unmarshal(data, &typed); err != nil {
			return nil, err
		}
		return typed, nil
	case EvaluationModelKindTypology:
		var typed TypologyModelPayload
		if err := json.Unmarshal(data, &typed); err != nil {
			return nil, err
		}
		return typed, nil
	case EvaluationModelKindBehavioralRating:
		var typed BehavioralRatingModelPayload
		if err := json.Unmarshal(data, &typed); err != nil {
			return nil, err
		}
		return typed, nil
	case EvaluationModelKindCognitive:
		var typed CognitiveModelPayload
		if err := json.Unmarshal(data, &typed); err != nil {
			return nil, err
		}
		return typed, nil
	default:
		return nil, nil
	}
}

// SnapshotFromReportInput decodes a frozen report input for Interpretation replay.
func SnapshotFromReportInput(data []byte, model ModelRef) (*InputSnapshot, error) {
	if len(data) == 0 {
		return nil, nil
	}
	decoded, err := decodeReportInputBytes(data, model.Kind.String())
	if err != nil {
		return nil, err
	}
	if decoded.Payload == nil {
		return nil, fmt.Errorf("report input payload is missing for kind %s", model.Kind)
	}
	snapshot := &InputSnapshot{
		Model: &ModelSnapshot{
			Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm,
			Code: model.Code, Version: model.Version, Title: model.Title, Payload: decoded.Payload,
		},
		ModelPayload:         decoded.Payload,
		InterpretationAssets: decoded.InterpretationAssets,
	}
	return snapshot, nil
}

// InterpretationAssetsFromSnapshot resolves frozen presentation assets from an
// evaluation input snapshot (report input envelope, scale payload field, etc.).
func InterpretationAssetsFromSnapshot(input *InputSnapshot) (interpretationassets.Assets, bool) {
	if input == nil {
		return interpretationassets.Assets{}, false
	}
	if input.InterpretationAssets != nil && input.InterpretationAssets.IsMaterialized() {
		return *input.InterpretationAssets, true
	}
	if scale, ok := ScalePayload(input); ok && scale.InterpretationAssets != nil && scale.InterpretationAssets.IsMaterialized() {
		return *scale.InterpretationAssets, true
	}
	if scale, ok := BehavioralRatingScaleSnapshot(input); ok && scale.InterpretationAssets != nil && scale.InterpretationAssets.IsMaterialized() {
		return *scale.InterpretationAssets, true
	}
	if scale, ok := CognitiveScaleSnapshot(input); ok && scale.InterpretationAssets != nil && scale.InterpretationAssets.IsMaterialized() {
		return *scale.InterpretationAssets, true
	}
	return interpretationassets.Assets{}, false
}
