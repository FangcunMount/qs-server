// Package evaluationfact is the read-only anti-corruption boundary through
// which other modules consume committed Evaluation outcomes. Unlike the
// retired alias package, it owns wrappers and adapters and does not re-export
// Evaluation types as aliases.
package evaluationfact

import (
	"context"
	"encoding/json"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ModelIdentity struct {
	Kind      modelcatalog.Kind
	SubKind   modelcatalog.SubKind
	Algorithm modelcatalog.Algorithm
	Code      string
	Version   string
	Title     string
}

type RuntimeIdentity struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	PayloadFormat   string
}

// Record is a read-only view over one committed Evaluation outcome.
type Record struct{ value *domainoutcome.Record }

func WrapRecord(value *domainoutcome.Record) *Record {
	if value == nil {
		return nil
	}
	return &Record{value: value}
}

func (r *Record) ID() meta.ID                  { return r.value.ID() }
func (r *Record) OrgID() int64                 { return r.value.OrgID() }
func (r *Record) AssessmentID() meta.ID        { return r.value.AssessmentID() }
func (r *Record) TesteeID() uint64             { return r.value.TesteeID() }
func (r *Record) RunID() string                { return r.value.RunID() }
func (r *Record) InputSnapshotRef() string     { return r.value.InputSnapshotRef() }
func (r *Record) SchemaVersion() uint          { return r.value.SchemaVersion() }
func (r *Record) Payload() json.RawMessage     { return r.value.Payload() }
func (r *Record) ReportInput() json.RawMessage { return r.value.ReportInput() }

func (r *Record) Model() ModelIdentity {
	v := r.value.Model()
	return ModelIdentity{Kind: v.Kind, SubKind: v.SubKind, Algorithm: v.Algorithm, Code: v.Code, Version: v.Version, Title: v.Title}
}

func (r *Record) Runtime() RuntimeIdentity {
	v := r.value.Runtime()
	return RuntimeIdentity{AlgorithmFamily: v.AlgorithmFamily, DecisionKind: v.DecisionKind, PayloadFormat: v.PayloadFormat}
}

// Execution is a distinct port type with the same immutable execution shape.
// It is deliberately a defined type, not an alias.
type Execution domainoutcome.Execution

// LegacyOutcome is a concrete transient preview contract. It exists only for
// in-process model preview and characterization; production reads Record.
type LegacyOutcome struct {
	Assessment           *assessment.Assessment
	Input                *evaluationinput.InputSnapshot
	Execution            *Execution
	RuntimeDescriptorKey evalpipeline.RuntimeDescriptorKey
}

func AdaptLegacyOutcome(value evaloutcome.Outcome) LegacyOutcome {
	return LegacyOutcome{
		Assessment: value.Assessment, Input: value.Input,
		Execution: (*Execution)(value.Execution), RuntimeDescriptorKey: value.RuntimeDescriptorKey,
	}
}

func RestoreExecution(record *Record) (*Execution, error) {
	value, err := evaloutcome.RestoreExecution(record.value)
	if err != nil {
		return nil, err
	}
	return (*Execution)(value), nil
}

func RestoreReportInput(record *Record) (*evaluationinput.InputSnapshot, error) {
	return evaloutcome.RestoreReportInput(record.value)
}

const (
	ScoreKindRawTotal     = domainoutcome.ScoreKindRawTotal
	ScoreKindMatchPercent = domainoutcome.ScoreKindMatchPercent
)

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
	var source any
	if detail, err := outcometypology.PersonalityTypeDetailFromPayload(payload); err == nil {
		source = detail
	} else if detail, err := typologylegacy.MBTIResultDetailFromPayload(payload); err == nil {
		source = typologylegacy.PersonalityTypeDetailFromMBTI(detail)
	} else if detail, err := typologylegacy.SBTIResultDetailFromPayload(payload); err == nil {
		source = typologylegacy.PersonalityTypeDetailFromSBTI(detail)
	} else {
		return PersonalityTypeDetail{}, false
	}
	var result PersonalityTypeDetail
	return result, transcode(source, &result)
}

func TraitProfileDetailFromPayload(payload any) (TraitProfileDetail, bool) {
	var source any
	if detail, err := outcometypology.TraitProfileDetailFromPayload(payload); err == nil {
		source = detail
	} else if detail, err := typologylegacy.BigFiveResultDetailFromPayload(payload); err == nil {
		source = typologylegacy.TraitProfileDetailFromBigFive(detail)
	} else {
		return TraitProfileDetail{}, false
	}
	var result TraitProfileDetail
	return result, transcode(source, &result)
}

func transcode(source any, target any) bool {
	payload, err := json.Marshal(source)
	if err != nil {
		return false
	}
	return json.Unmarshal(payload, target) == nil
}

// Repository is intentionally read-only. Interpretation cannot save or mutate
// Evaluation facts through this boundary.
type Repository interface {
	FindByID(ctx context.Context, id meta.ID) (*Record, error)
	FindByAssessmentID(ctx context.Context, assessmentID meta.ID) (*Record, error)
}

type repositoryAdapter struct{ source domainoutcome.Repository }

func AdaptRepository(source domainoutcome.Repository) Repository {
	if source == nil {
		return nil
	}
	return &repositoryAdapter{source: source}
}

func (a *repositoryAdapter) FindByID(ctx context.Context, id meta.ID) (*Record, error) {
	record, err := a.source.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return WrapRecord(record), nil
}

func (a *repositoryAdapter) FindByAssessmentID(ctx context.Context, assessmentID meta.ID) (*Record, error) {
	record, err := a.source.FindByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return WrapRecord(record), nil
}
