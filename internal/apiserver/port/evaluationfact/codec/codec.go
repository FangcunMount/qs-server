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

const (
	schemaV1 uint = 1
	schemaV2 uint = 2
)

func DecodeExecution(record *evaluationfact.Record) (*evaluationfact.Execution, error) {
	if record == nil {
		return nil, fmt.Errorf("evaluation outcome is required")
	}
	if record.SchemaVersion() != 0 && record.SchemaVersion() != schemaV1 && record.SchemaVersion() != schemaV2 {
		return nil, fmt.Errorf("unsupported evaluation outcome schema version %d", record.SchemaVersion())
	}
	execution, err := decodeExecution(record.Payload(), record.Model(), record.Runtime(), record.SchemaVersion())
	if err != nil {
		return nil, fmt.Errorf("decode evaluation outcome %s: %w", record.ID(), err)
	}
	return execution, nil
}

// DecodeTransientExecution applies the fact codec to Preview output without
// manufacturing a committed Evaluation record.
func DecodeTransientExecution(value any, model evaluationfact.ModelIdentity, runtime evaluationfact.RuntimeIdentity) (*evaluationfact.Execution, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode transient evaluation execution: %w", err)
	}
	return decodeExecution(payload, model, runtime, schemaV1)
}

func decodeExecution(payload []byte, model evaluationfact.ModelIdentity, runtime evaluationfact.RuntimeIdentity, schema uint) (*evaluationfact.Execution, error) {
	var execution evaluationfact.Execution
	if err := json.Unmarshal(payload, &execution); err != nil {
		return nil, err
	}
	execution.ModelRef = evaluationfact.ModelRef{
		ModelKind: model.Kind, ModelSubKind: model.SubKind, ModelAlgorithm: model.Algorithm,
		ModelCode: model.Code, ModelVersion: model.Version, ModelTitle: model.Title,
	}
	if schema == schemaV2 {
		if err := restoreV2TypedDetail(payload, runtime, &execution); err != nil {
			return nil, err
		}
	} else if err := restoreTypedDetail(payload, model, runtime, &execution); err != nil {
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

func restoreV2TypedDetail(payload []byte, runtime evaluationfact.RuntimeIdentity, execution *evaluationfact.Execution) error {
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
		return nil, nil
	}
	var payload evaluationinput.ModelPayload
	switch record.Model().Kind {
	case modelcatalog.KindScale:
		var typed evaluationinput.ScaleModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, decodeReportInputError(record, err)
		}
		payload = typed
	case modelcatalog.KindTypology:
		var typed evaluationinput.TypologyModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, decodeReportInputError(record, err)
		}
		payload = typed
	case modelcatalog.KindBehavioralRating:
		var typed evaluationinput.BehavioralRatingModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, decodeReportInputError(record, err)
		}
		payload = typed
	case modelcatalog.KindCognitive:
		var typed evaluationinput.CognitiveModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, decodeReportInputError(record, err)
		}
		payload = typed
	default:
		return nil, fmt.Errorf("unsupported report input model kind %s", record.Model().Kind)
	}
	model := record.Model()
	snapshot := &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKind(model.Kind), SubKind: string(model.SubKind), Algorithm: string(model.Algorithm), Code: model.Code, Version: model.Version, Title: model.Title, Payload: payload}
	return &evaluationinput.InputSnapshot{Model: snapshot, ModelPayload: payload}, nil
}

func decodeReportInputError(record *evaluationfact.Record, err error) error {
	return fmt.Errorf("decode report input %s: %w", record.ID(), err)
}

func restoreTypedDetail(payload []byte, model evaluationfact.ModelIdentity, runtime evaluationfact.RuntimeIdentity, execution *evaluationfact.Execution) error {
	var wire struct {
		Detail struct{ Payload json.RawMessage }
	}
	if err := json.Unmarshal(payload, &wire); err != nil || len(wire.Detail.Payload) == 0 || string(wire.Detail.Payload) == "null" {
		return err
	}
	switch runtime.DecisionKind {
	case modelcatalog.DecisionKindPoleComposition, modelcatalog.DecisionKindNearestPattern:
		detail, err := decodePersonalityTypeDetail(wire.Detail.Payload, model.Algorithm)
		if err != nil {
			return err
		}
		execution.Detail.Payload = detail
	case modelcatalog.DecisionKindTraitProfile:
		detail, err := decodeTraitProfileDetail(wire.Detail.Payload)
		if err != nil {
			return err
		}
		execution.Detail.Payload = detail
	default:
		if detail, err := decodePersonalityTypeDetail(wire.Detail.Payload, model.Algorithm); err == nil && detail.TypeCode != "" {
			execution.Detail.Payload = detail
			return nil
		}
		if detail, err := decodeTraitProfileDetail(wire.Detail.Payload); err == nil && len(detail.Traits) > 0 {
			execution.Detail.Payload = detail
			return nil
		}
		return restoreLegacyFactorScores(wire.Detail.Payload, execution)
	}
	return nil
}

func restoreLegacyFactorScores(payload []byte, execution *evaluationfact.Execution) error {
	var factors []legacyFactorScoreWire
	if err := json.Unmarshal(payload, &factors); err != nil {
		return err
	}
	if len(execution.Dimensions) == 0 {
		execution.Dimensions = make([]evaluationfact.DimensionResult, 0, len(factors))
		for _, factor := range factors {
			dimension := evaluationfact.DimensionResult{Code: factor.FactorCode, Name: factor.FactorName, Kind: evaluationfact.DimensionKindFactor, Score: &evaluationfact.ScoreValue{Kind: evaluationfact.ScoreKindRawTotal, Value: factor.RawScore}}
			if factor.IsTotalScore {
				dimension.Role = "total"
			}
			if factor.RiskLevel != "" {
				dimension.Level = &evaluationfact.ResultLevel{Code: factor.RiskLevel, Label: factor.RiskLevel}
			}
			execution.Dimensions = append(execution.Dimensions, dimension)
		}
	}
	execution.Detail.Payload = nil
	return nil
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

func decodePersonalityTypeDetail(payload []byte, _ modelcatalog.Algorithm) (PersonalityTypeDetail, error) {
	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(payload, &envelope)
	if _, legacyMBTI := envelope["profile"]; legacyMBTI {
		var wire mbtiDetailWire
		if err := json.Unmarshal(payload, &wire); err != nil {
			return PersonalityTypeDetail{}, err
		}
		return personalityTypeFromMBTIWire(wire), nil
	}
	var result PersonalityTypeDetail
	if err := json.Unmarshal(payload, &result); err != nil || result.TypeCode == "" {
		return result, fmt.Errorf("unsupported personality detail payload")
	}
	if result.MatchPercent == 0 && result.Similarity > 0 {
		result.MatchPercent = result.Similarity * 100
	}
	if result.Similarity == 0 && result.MatchPercent > 0 {
		result.Similarity = result.MatchPercent / 100
	}
	if result.Outcome.Code != "" {
		result.IsSpecial = result.IsSpecial || result.Outcome.IsSpecial
		if result.Commentary == "" {
			result.Commentary = result.Outcome.Commentary
		}
	}
	return result, nil
}

func decodeTraitProfileDetail(payload []byte) (TraitProfileDetail, error) {
	var result TraitProfileDetail
	if err := json.Unmarshal(payload, &result); err != nil || len(result.Traits) == 0 {
		return result, fmt.Errorf("unsupported trait profile detail payload")
	}
	return result, nil
}

type mbtiDimensionWire struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	LeftPole   string  `json:"left_pole"`
	RightPole  string  `json:"right_pole"`
	RawScore   float64 `json:"raw_score"`
	Preference string  `json:"preference"`
	Strength   float64 `json:"strength"`
}
type mbtiProfileWire struct {
	Summary     string   `json:"summary"`
	Strengths   []string `json:"strengths"`
	Weaknesses  []string `json:"weaknesses"`
	Suggestions []string `json:"suggestions"`
}
type mbtiSourceWire struct {
	Attribution   string `json:"attribution"`
	License       string `json:"license"`
	NonCommercial bool   `json:"non_commercial"`
}
type mbtiDetailWire struct {
	TypeCode     string              `json:"type_code"`
	TypeName     string              `json:"type_name"`
	OneLiner     string              `json:"one_liner"`
	MatchPercent float64             `json:"match_percent"`
	ImageURL     string              `json:"image_url"`
	Dimensions   []mbtiDimensionWire `json:"dimensions"`
	Profile      mbtiProfileWire     `json:"profile"`
	Source       mbtiSourceWire      `json:"source"`
}

func personalityTypeFromMBTIWire(wire mbtiDetailWire) PersonalityTypeDetail {
	dimensions := make([]PersonalityDimensionResult, 0, len(wire.Dimensions))
	for _, dim := range wire.Dimensions {
		dimensions = append(dimensions, PersonalityDimensionResult{Code: dim.Code, Name: dim.Name, LeftPole: dim.LeftPole, RightPole: dim.RightPole, RawScore: dim.RawScore, Preference: dim.Preference, Strength: dim.Strength})
	}
	return PersonalityTypeDetail{
		TypeCode: wire.TypeCode, TypeName: wire.TypeName, OneLiner: wire.OneLiner, Summary: wire.Profile.Summary, MatchPercent: wire.MatchPercent, Similarity: wire.MatchPercent / 100, ImageURL: wire.ImageURL,
		Dimensions: dimensions, Strengths: append([]string(nil), wire.Profile.Strengths...), Weaknesses: append([]string(nil), wire.Profile.Weaknesses...), Suggestions: append([]string(nil), wire.Profile.Suggestions...),
		Outcome: modeltypology.Outcome{Code: wire.TypeCode, Name: wire.TypeName, OneLiner: wire.OneLiner, Summary: wire.Profile.Summary},
		Source:  modeltypology.Source{Attribution: wire.Source.Attribution, License: wire.Source.License, NonCommercial: wire.Source.NonCommercial},
	}
}

// legacyFactorScoreWire is the schema-v1 decoder retained until production
// data audit confirms no historical scale Outcome requires it.
type legacyFactorScoreWire struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    string
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}
