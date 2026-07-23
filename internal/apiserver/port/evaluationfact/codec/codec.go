// Package codec owns versioned decoding of immutable Evaluation facts.
package codec

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

const currentOutcomeSchema uint = 2

func DecodeExecution(record *evaluationfact.Record) (*evaluationfact.Execution, error) {
	if record == nil {
		return nil, fmt.Errorf("evaluation outcome is required")
	}
	schema := record.SchemaVersion()
	if schema != currentOutcomeSchema {
		return nil, fmt.Errorf("unsupported evaluation outcome schema version %d", schema)
	}
	execution, err := decodeExecution(record.Payload(), record.Model(), record.Runtime())
	if err != nil {
		return nil, fmt.Errorf("decode evaluation outcome %s: %w", record.ID(), err)
	}
	return execution, nil
}

func decodeExecution(payload []byte, model evaluationfact.ModelIdentity, runtime evaluationfact.RuntimeIdentity) (*evaluationfact.Execution, error) {
	var execution evaluationfact.Execution
	if err := json.Unmarshal(payload, &execution); err != nil {
		return nil, err
	}
	execution.ModelRef = evaluationfact.ModelRef{
		ModelKind: model.Kind, ModelSubKind: model.SubKind, ModelAlgorithm: model.Algorithm,
		ModelCode: model.Code, ModelVersion: model.Version, ModelTitle: model.Title,
	}
	if err := restoreCurrentTypedDetail(payload, runtime, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

// ClassificationFact is the schema-v2 typology fact contract. It has no
// report prose or presentation assets.
type ClassificationFact struct {
	TypeCode       string  `json:"type_code"`
	Pattern        string  `json:"pattern,omitempty"`
	MatchPercent   float64 `json:"match_percent,omitempty"`
	Similarity     float64 `json:"similarity,omitempty"`
	SpecialTrigger string  `json:"special_trigger,omitempty"`
	IsSpecial      bool    `json:"is_special,omitempty"`
}

func ClassificationFactFromPayload(payload any) (ClassificationFact, bool) {
	fact, ok := payload.(ClassificationFact)
	if ok {
		return fact, true
	}
	if pointer, ok := payload.(*ClassificationFact); ok && pointer != nil {
		return *pointer, true
	}
	return ClassificationFact{}, false
}

func restoreCurrentTypedDetail(payload []byte, runtime evaluationfact.RuntimeIdentity, execution *evaluationfact.Execution) error {
	if runtime.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorClassification {
		execution.Detail.Payload = nil
		return nil
	}
	if runtime.DecisionKind == modelcatalog.DecisionKindTraitProfile {
		execution.Detail.Payload = nil
		return nil
	}
	var wire struct {
		Detail struct{ Payload json.RawMessage }
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		return err
	}
	if len(wire.Detail.Payload) == 0 || string(wire.Detail.Payload) == "null" {
		return fmt.Errorf("schema v2 typology classification fact is missing")
	}
	var fact ClassificationFact
	if err := json.Unmarshal(wire.Detail.Payload, &fact); err != nil {
		return err
	}
	if fact.TypeCode == "" {
		return fmt.Errorf("schema v2 typology classification code is missing")
	}
	execution.Detail.Payload = fact
	return nil
}

func DecodeReportInput(record *evaluationfact.Record) (*evaluationinput.InputSnapshot, error) {
	if record == nil {
		return nil, fmt.Errorf("evaluation outcome is required")
	}
	if len(record.ReportInput()) == 0 {
		return nil, decodeReportInputError(record, fmt.Errorf("report input is required"))
	}
	model := record.Model()
	snapshot, err := evaluationinput.SnapshotFromReportInput(record.ReportInput(), evaluationinput.ModelRef{
		Kind:      evaluationinput.EvaluationModelKind(model.Kind),
		Algorithm: string(model.Algorithm), Code: model.Code, Version: model.Version, Title: model.Title,
	})
	if err != nil {
		return nil, decodeReportInputError(record, err)
	}
	return snapshot, nil
}

func decodeReportInputError(record *evaluationfact.Record, err error) error {
	return fmt.Errorf("decode report input %s: %w", record.ID(), err)
}

type PersonalityDimensionResult struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Model      string  `json:"model,omitempty"`
	LeftPole   string  `json:"left_pole,omitempty"`
	RightPole  string  `json:"right_pole,omitempty"`
	RawScore   float64 `json:"raw_score"`
	Preference string  `json:"preference,omitempty"`
	Strength   float64 `json:"strength,omitempty"`
	Level      string  `json:"level,omitempty"`
}

type PersonalityTypeDetail struct {
	TypeCode       string                       `json:"type_code"`
	TypeName       string                       `json:"type_name"`
	OneLiner       string                       `json:"one_liner,omitempty"`
	Summary        string                       `json:"summary,omitempty"`
	Pattern        string                       `json:"pattern,omitempty"`
	MatchPercent   float64                      `json:"match_percent,omitempty"`
	Similarity     float64                      `json:"similarity,omitempty"`
	ImageURL       string                       `json:"image_url,omitempty"`
	Rarity         modeltypology.Rarity         `json:"rarity,omitempty"`
	Dimensions     []PersonalityDimensionResult `json:"dimensions,omitempty"`
	Strengths      []string                     `json:"strengths,omitempty"`
	Weaknesses     []string                     `json:"weaknesses,omitempty"`
	Suggestions    []string                     `json:"suggestions,omitempty"`
	Outcome        modeltypology.Outcome        `json:"outcome,omitempty"`
	Source         modeltypology.Source         `json:"source,omitempty"`
	SpecialTrigger string                       `json:"special_trigger,omitempty"`
	IsSpecial      bool                         `json:"is_special,omitempty"`
	Commentary     string                       `json:"commentary,omitempty"`
}

type TraitProfileFactorResult struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	RawScore float64 `json:"raw_score"`
}
type TraitProfileDetail struct {
	Traits []TraitProfileFactorResult `json:"traits"`
	Source modeltypology.Source       `json:"source,omitempty"`
}

func PersonalityTypeDetailFromPayload(payload any) (PersonalityTypeDetail, bool) {
	if detail, ok := payload.(PersonalityTypeDetail); ok {
		return detail, true
	}
	if detail, ok := payload.(*PersonalityTypeDetail); ok && detail != nil {
		return *detail, true
	}
	return PersonalityTypeDetail{}, false
}

func TraitProfileDetailFromPayload(payload any) (TraitProfileDetail, bool) {
	if detail, ok := payload.(TraitProfileDetail); ok {
		return detail, true
	}
	if detail, ok := payload.(*TraitProfileDetail); ok && detail != nil {
		return *detail, true
	}
	return TraitProfileDetail{}, false
}
