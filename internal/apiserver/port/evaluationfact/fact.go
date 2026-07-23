// Package evaluationfact defines the immutable, read-only contract through
// which downstream modules consume committed Evaluation facts.
package evaluationfact

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ModelIdentity struct {
	Kind      modelcatalog.Kind
	Algorithm modelcatalog.Algorithm
	Code      string
	Version   string
	Title     string
}

type RuntimeIdentity struct {
	DecisionKind modelcatalog.DecisionKind
}

type NewRecordInput struct {
	ID               meta.ID
	OrgID            int64
	AssessmentID     meta.ID
	TesteeID         uint64
	RunID            string
	Model            ModelIdentity
	Runtime          RuntimeIdentity
	InputSnapshotRef string
	SchemaVersion    uint
	Payload          json.RawMessage
	ReportInput      json.RawMessage
	EvaluatedAt      time.Time
}

// Record is an immutable copy of one committed Evaluation outcome.
type Record struct {
	id               meta.ID
	orgID            int64
	assessmentID     meta.ID
	testeeID         uint64
	runID            string
	model            ModelIdentity
	runtime          RuntimeIdentity
	inputSnapshotRef string
	schemaVersion    uint
	payload          json.RawMessage
	reportInput      json.RawMessage
	evaluatedAt      time.Time
}

func NewRecord(input NewRecordInput) *Record {
	return &Record{
		id: input.ID, orgID: input.OrgID, assessmentID: input.AssessmentID,
		testeeID: input.TesteeID, runID: input.RunID, model: input.Model,
		runtime: input.Runtime, inputSnapshotRef: input.InputSnapshotRef,
		schemaVersion: input.SchemaVersion, payload: cloneBytes(input.Payload),
		reportInput: cloneBytes(input.ReportInput), evaluatedAt: input.EvaluatedAt,
	}
}

func (r *Record) ID() meta.ID              { return r.id }
func (r *Record) OrgID() int64             { return r.orgID }
func (r *Record) AssessmentID() meta.ID    { return r.assessmentID }
func (r *Record) TesteeID() uint64         { return r.testeeID }
func (r *Record) RunID() string            { return r.runID }
func (r *Record) Model() ModelIdentity     { return r.model }
func (r *Record) Runtime() RuntimeIdentity { return r.runtime }
func (r *Record) InputSnapshotRef() string { return r.inputSnapshotRef }
func (r *Record) SchemaVersion() uint      { return r.schemaVersion }
func (r *Record) EvaluatedAt() time.Time   { return r.evaluatedAt }
func (r *Record) Payload() json.RawMessage { return cloneBytes(r.payload) }
func (r *Record) ReportInput() json.RawMessage {
	return cloneBytes(r.reportInput)
}

func cloneBytes(value []byte) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

// Execution is the version-neutral decoded shape of an Evaluation fact.
type Execution struct {
	ModelRef   ModelRef
	Summary    Summary
	Detail     Detail
	Primary    *ScoreValue
	Level      *ResultLevel
	Profile    *ProfileResult
	Dimensions []DimensionResult
	Validity   []ValidityResult
}

type ModelRef struct {
	ModelKind      modelcatalog.Kind      `json:"kind"`
	ModelAlgorithm modelcatalog.Algorithm `json:"algorithm,omitempty"`
	ModelCode      string                 `json:"code"`
	ModelVersion   string                 `json:"version,omitempty"`
	ModelTitle     string                 `json:"title,omitempty"`
}

func (r ModelRef) IsEmpty() bool                     { return r.ModelKind == "" && r.ModelCode == "" }
func (r ModelRef) Kind() modelcatalog.Kind           { return r.ModelKind }
func (r ModelRef) Algorithm() modelcatalog.Algorithm { return r.ModelAlgorithm }
func (r ModelRef) Code() meta.Code                   { return meta.NewCode(r.ModelCode) }
func (r ModelRef) Version() string                   { return r.ModelVersion }
func (r ModelRef) Title() string                     { return r.ModelTitle }

type Summary struct {
	PrimaryLabel string
	Score        *float64
	Level        *string
	Tags         []string
}

type Detail struct {
	Kind    modelcatalog.Kind
	Payload any
}

type DimensionKind string
type ScoreKind string
type ProfileKind string

const (
	DimensionKindFactor  DimensionKind = "factor"
	DimensionKindPole    DimensionKind = "pole"
	DimensionKindTrait   DimensionKind = "trait"
	DimensionKindIndex   DimensionKind = "index"
	DimensionKindAbility DimensionKind = "ability"

	ScoreKindRawTotal      ScoreKind = "raw_total"
	ScoreKindMatchPercent  ScoreKind = "match_percent"
	ScoreKindTScore        ScoreKind = "t_score"
	ScoreKindPercentile    ScoreKind = "percentile"
	ScoreKindStandardScore ScoreKind = "standard_score"

	ProfileKindPersonalityType  ProfileKind = "personality_type"
	ProfileKindPersonalityTrait ProfileKind = "personality_trait"
	ProfileKindAbilityProfile   ProfileKind = "ability_profile"
)

type ScoreValue struct {
	Kind  ScoreKind
	Value float64
	Label string
	Max   *float64
}

type ResultLevel struct {
	Code     string
	Label    string
	Severity string
}

type NormReference struct {
	ScoreKind    ScoreKind
	Benchmark    float64
	TableVersion string
	FormVariant  string
	MinAgeMonths int
	MaxAgeMonths int
	Gender       string
}

type ProfileResult struct {
	Kind   ProfileKind
	Code   string
	Name   string
	Traits []string
}

type DimensionResult struct {
	Code           string
	Name           string
	Kind           DimensionKind
	Role           string
	ParentCode     string
	HierarchyLevel int
	SortOrder      int
	Score          *ScoreValue
	DerivedScores  []ScoreValue
	Level          *ResultLevel
	NormReference  *NormReference
	Preference     string
	Strength       *float64
	LeftPole       string
	RightPole      string
	Model          string
}

type ValidityResult struct {
	Code    string
	Label   string
	Passed  bool
	Message string
}

// Repository intentionally exposes no mutation operations.
type Repository interface {
	FindByID(ctx context.Context, id meta.ID) (*Record, error)
	FindByAssessmentID(ctx context.Context, assessmentID meta.ID) (*Record, error)
}
