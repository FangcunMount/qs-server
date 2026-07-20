package evaluationinput

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

const (
	// ReportInputSchemaLegacy is the historical shape: raw ModelPayload JSON only.
	ReportInputSchemaLegacy = 0
	// ReportInputSchemaV2 wraps payload with a frozen InterpretationAssets snapshot (MC-R016).
	ReportInputSchemaV2 = 2
	// ReportInputSchemaV3 freezes minimal InterpretationAssets + factor catalog without ModelPayload (MC-R017).
	ReportInputSchemaV3 = 3
)

type reportInputEnvelope struct {
	SchemaVersion        uint                         `json:"schema_version"`
	Payload              json.RawMessage              `json:"payload,omitempty"`
	InterpretationAssets *interpretationassets.Assets `json:"InterpretationAssets,omitempty"`
	ModelRef             *ModelRef                    `json:"model_ref,omitempty"`
	FactorCatalog        []FactorCatalogEntry         `json:"factor_catalog,omitempty"`
	TypologySource       *typology.Source             `json:"typology_source,omitempty"`
	Norming              *NormingFreeze               `json:"norming,omitempty"`
}

// MarshalReportInput freezes evaluation report input. New scale publishes prefer schema
// v3 when assets and factor catalog are sufficient; otherwise v2 or legacy payload-only.
func MarshalReportInput(opts ReportInputFreezeOptions) ([]byte, error) {
	if opts.Payload == nil && (opts.Assets == nil || !opts.Assets.IsMaterialized()) {
		return nil, fmt.Errorf("report input payload or interpretation assets is required")
	}
	if CanFreezeMinimalReportInput(opts) {
		return json.Marshal(reportInputEnvelope{
			SchemaVersion:        ReportInputSchemaV3,
			InterpretationAssets: opts.Assets,
			ModelRef:             &opts.ModelRef,
			FactorCatalog:        opts.FactorCatalog,
			TypologySource:       opts.TypologySource,
			Norming:              opts.Norming,
		})
	}
	rawPayload, err := json.Marshal(opts.Payload)
	if err != nil {
		return nil, err
	}
	if opts.Assets == nil || !opts.Assets.IsMaterialized() {
		return rawPayload, nil
	}
	return json.Marshal(reportInputEnvelope{
		SchemaVersion:        ReportInputSchemaV2,
		Payload:              rawPayload,
		InterpretationAssets: opts.Assets,
	})
}

type decodedReportInput struct {
	Payload              ModelPayload
	InterpretationAssets *interpretationassets.Assets
	ModelRef             ModelRef
	FactorCatalog        []FactorCatalogEntry
	TypologySource       *typology.Source
	Norming              *NormingFreeze
	SchemaVersion        uint
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
		decoded := decodedReportInput{
			InterpretationAssets: envelope.InterpretationAssets,
			FactorCatalog:        envelope.FactorCatalog,
			TypologySource:       envelope.TypologySource,
			Norming:              envelope.Norming,
			SchemaVersion:        envelope.SchemaVersion,
		}
		if envelope.ModelRef != nil {
			decoded.ModelRef = *envelope.ModelRef
		}
		if envelope.SchemaVersion >= ReportInputSchemaV3 {
			return decoded, nil
		}
		payload, err := decodeModelPayloadBytes(envelope.Payload, kind)
		if err != nil {
			return decodedReportInput{}, err
		}
		decoded.Payload = payload
		return decoded, nil
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
	ref := model
	if decoded.ModelRef.Kind != "" {
		ref = decoded.ModelRef
	}
	snapshot := &InputSnapshot{InterpretationAssets: decoded.InterpretationAssets}
	if decoded.SchemaVersion >= ReportInputSchemaV3 {
		return snapshotFromMinimalReportInput(model, decoded)
	}
	if decoded.Payload == nil {
		return nil, fmt.Errorf("report input payload is missing for kind %s", model.Kind)
	}
	snapshot.Model = &ModelSnapshot{
		Kind: ref.Kind, SubKind: ref.SubKind, Algorithm: ref.Algorithm,
		Code: ref.Code, Version: ref.Version, Title: ref.Title, Payload: decoded.Payload,
	}
	snapshot.ModelPayload = decoded.Payload
	return snapshot, nil
}
