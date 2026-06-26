package assessmentmodel

import (
	"encoding/json"
	"fmt"
)

// Kind is the canonical assessment model family.
type Kind string

const (
	KindScale            Kind = "scale"
	KindPersonality      Kind = "personality"
	KindBehavioralRating Kind = "behavioral_rating"
	KindCognitive        Kind = "cognitive"
	KindCustom           Kind = "custom"

	// Migration-only flat kinds read from legacy envelopes; do not use in new writes.
	KindMBTIMigration Kind = "mbti"
	KindSBTIMigration Kind = "sbti"
)

// RuleSetKind is kept as a compatibility name while callers migrate to Kind.
type RuleSetKind = Kind

const (
	RuleSetKindScale = KindScale
	RuleSetKindMBTI  = KindMBTIMigration
	RuleSetKindSBTI  = KindSBTIMigration
)

// SubKind narrows a Kind when multiple payload shapes share the same family.
type SubKind string

const (
	SubKindEmpty    SubKind = ""
	SubKindTrait    SubKind = "trait"
	SubKindTypology SubKind = "typology"
)

// Algorithm selects the evaluation algorithm within a model family.
type Algorithm string

const (
	AlgorithmScaleDefault       Algorithm = "scale_default"
	AlgorithmPersonalityTypology Algorithm = "personality_typology"
	AlgorithmBigFive            Algorithm = "bigfive"
	AlgorithmMBTI         Algorithm = "mbti"
	AlgorithmSBTI         Algorithm = "sbti"
	AlgorithmBrief2       Algorithm = "brief2"
	AlgorithmSPM          Algorithm = "spm"
)

func (k Kind) String() string { return string(k) }

func (k Kind) IsValid() bool {
	switch k {
	case KindScale, KindPersonality, KindBehavioralRating, KindCognitive, KindCustom,
		KindMBTIMigration, KindSBTIMigration:
		return true
	default:
		return false
	}
}

func (s SubKind) String() string { return string(s) }

func (a Algorithm) String() string { return string(a) }

// DecisionKind describes how raw scores map to outcomes.
type DecisionKind string

const (
	DecisionKindScoreRange      DecisionKind = "score_range"
	DecisionKindPoleComposition DecisionKind = "pole_composition"
	DecisionKindTraitProfile    DecisionKind = "trait_profile"
	DecisionKindNearestPattern  DecisionKind = "nearest_pattern"
	DecisionKindNormLookup      DecisionKind = "norm_lookup"
	DecisionKindAbilityLevel    DecisionKind = "ability_level"

	// Deprecated: use DecisionKindScoreRange.
	DecisionKindScoreRangeInterpretation DecisionKind = "score_range_interpretation"
)

const (
	SchemaVersionV1 = "1"
	SchemaVersionV2 = "2"

	// v2 production payload formats.
	PayloadFormatAssessmentScaleV1         = "assessmentmodel.scale.v1"
	PayloadFormatPersonalityTypologyV1     = "assessmentmodel.personality.typology.v1"
	PayloadFormatBehavioralRatingDefaultV1 = "assessmentmodel.behavioral_rating.default.v1"
	PayloadFormatCognitiveDefaultV1        = "assessmentmodel.cognitive.default.v1"

	// Legacy read-only payload formats (migration / outbox drain).
	PayloadFormatScaleV1 = "ruleset.scale.v1"
	PayloadFormatMBTIV1  = "ruleset.mbti.v1"
	PayloadFormatSBTIV1  = "ruleset.sbti.v1"

	PayloadFormatScaleV1Legacy = "evaluationinput.scale.v1"
	PayloadFormatMBTIV1Legacy  = "evaluationinput.mbti.v1"
	PayloadFormatSBTIV1Legacy  = "evaluationinput.sbti.v1"
)

// QuestionnaireBinding binds a published model to a questionnaire version.
type QuestionnaireBinding struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
}

// ModelDefinition is canonical published-model metadata.
type ModelDefinition struct {
	Kind      Kind
	SubKind   SubKind
	Algorithm Algorithm
	Code      string
	Version   string
	Title     string
	Status    string
}

// Definition is kept as a compatibility name while callers migrate to ModelDefinition.
type Definition struct {
	Kind    Kind
	Code    string
	Version string
	Title   string
	Status  string
}

// RuleSetDefinition is kept as a compatibility name while callers migrate to Definition.
type RuleSetDefinition = Definition

// DecisionSpec captures the outcome decision strategy for a published model.
type DecisionSpec struct {
	Kind DecisionKind
}

// SourceRef carries optional provenance metadata for a published snapshot.
type SourceRef map[string]any

// PublishedModelSnapshot is the v2 published-model envelope.
type PublishedModelSnapshot struct {
	SchemaVersion string
	Model         ModelDefinition
	Binding       QuestionnaireBinding
	Decision      DecisionSpec
	Source        SourceRef
	PayloadFormat string
	Payload       []byte
}

// Snapshot is the v1 envelope kept for migration readers.
type Snapshot struct {
	SchemaVersion string
	PayloadFormat string
	Definition    Definition
	Binding       QuestionnaireBinding
	DecisionKind  DecisionKind
	Source        map[string]any
	Payload       []byte
}

// RuleSetSnapshot is kept as a compatibility name while callers migrate to Snapshot.
type RuleSetSnapshot = Snapshot

const RuleSetSchemaVersionV1 = SchemaVersionV1

func IsScalePayloadFormat(format string) bool {
	switch format {
	case PayloadFormatAssessmentScaleV1,
		PayloadFormatScaleV1, PayloadFormatScaleV1Legacy:
		return true
	default:
		return false
	}
}

// IsMBTIPayloadFormat reports legacy MBTI payload formats only.
// v2 typology payloads must be distinguished by AlgorithmFromTypologyPayload.
func IsMBTIPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatMBTIV1, PayloadFormatMBTIV1Legacy:
		return true
	default:
		return false
	}
}

// IsSBTIPayloadFormat reports legacy SBTI payload formats only.
// v2 typology payloads must be distinguished by AlgorithmFromTypologyPayload.
func IsSBTIPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatSBTIV1, PayloadFormatSBTIV1Legacy:
		return true
	default:
		return false
	}
}

func IsPersonalityTypologyPayloadFormat(format string) bool {
	return format == PayloadFormatPersonalityTypologyV1
}

type typologyAlgorithmEnvelope struct {
	Algorithm Algorithm `json:"algorithm"`
}

// AlgorithmFromTypologyPayload reads the algorithm identity from a v2 typology payload.
func AlgorithmFromTypologyPayload(payload []byte) (Algorithm, error) {
	var envelope typologyAlgorithmEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("decode typology payload algorithm: %w", err)
	}
	if envelope.Algorithm == "" {
		return "", fmt.Errorf("typology payload algorithm is empty")
	}
	return envelope.Algorithm, nil
}

// LegacyKindMapping resolves deprecated flat kinds to v2 identity triples.
func LegacyKindMapping(kind Kind) (Kind, SubKind, Algorithm, bool) {
	switch kind {
	case KindScale:
		return KindScale, SubKindEmpty, AlgorithmScaleDefault, true
	case KindMBTIMigration:
		return KindPersonality, SubKindTypology, AlgorithmMBTI, true
	case KindSBTIMigration:
		return KindPersonality, SubKindTypology, AlgorithmSBTI, true
	default:
		return "", "", "", false
	}
}

// ModelDefinitionFromLegacy builds a v2 definition from a v1 envelope definition.
func ModelDefinitionFromLegacy(def Definition, decision DecisionKind) ModelDefinition {
	if kind, subKind, algorithm, ok := LegacyKindMapping(def.Kind); ok {
		return ModelDefinition{
			Kind:      kind,
			SubKind:   subKind,
			Algorithm: algorithm,
			Code:      def.Code,
			Version:   def.Version,
			Title:     def.Title,
			Status:    def.Status,
		}
	}
	return ModelDefinition{
		Kind:    def.Kind,
		Code:    def.Code,
		Version: def.Version,
		Title:   def.Title,
		Status:  def.Status,
	}
}

// PublishedFromLegacy converts a v1 snapshot envelope to v2.
func PublishedFromLegacy(snapshot *Snapshot) *PublishedModelSnapshot {
	if snapshot == nil {
		return nil
	}
	source := SourceRef(nil)
	if snapshot.Source != nil {
		source = SourceRef(snapshot.Source)
	}
	return &PublishedModelSnapshot{
		SchemaVersion: SchemaVersionV2,
		Model:         ModelDefinitionFromLegacy(snapshot.Definition, snapshot.DecisionKind),
		Binding:       snapshot.Binding,
		Decision:      DecisionSpec{Kind: snapshot.DecisionKind},
		Source:        source,
		PayloadFormat: snapshot.PayloadFormat,
		Payload:       snapshot.Payload,
	}
}

// LegacyFromPublished converts a v2 snapshot to the v1 envelope for migration readers.
func LegacyFromPublished(snapshot *PublishedModelSnapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	def := Definition{
		Code:    snapshot.Model.Code,
		Version: snapshot.Model.Version,
		Title:   snapshot.Model.Title,
		Status:  snapshot.Model.Status,
	}
	switch {
	case snapshot.Model.Kind == KindPersonality && snapshot.Model.Algorithm == AlgorithmMBTI:
		def.Kind = KindMBTIMigration
	case snapshot.Model.Kind == KindPersonality && snapshot.Model.Algorithm == AlgorithmSBTI:
		def.Kind = KindSBTIMigration
	default:
		def.Kind = snapshot.Model.Kind
	}
	source := map[string]any(nil)
	if snapshot.Source != nil {
		source = map[string]any(snapshot.Source)
	}
	return &Snapshot{
		SchemaVersion: snapshot.SchemaVersion,
		PayloadFormat: snapshot.PayloadFormat,
		Definition:    def,
		Binding:       snapshot.Binding,
		DecisionKind:  snapshot.Decision.Kind,
		Source:        source,
		Payload:       snapshot.Payload,
	}
}
