package outcome

import (
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Execution is the canonical in-memory result returned by an Evaluator.
//
// It is intentionally distinct in role from Record: Execution is mutable
// while a run is being calculated; Record is the immutable, durable fact
// created only after the Evaluation commit succeeds.
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

// ModelRef is the model identity captured by an execution result. It does not
// retain Assessment-owned storage identifiers.
type ModelRef struct {
	ModelKind      modelcatalog.Kind      `json:"kind"`
	ModelSubKind   modelcatalog.SubKind   `json:"sub_kind,omitempty"`
	ModelAlgorithm modelcatalog.Algorithm `json:"algorithm,omitempty"`
	ModelCode      string                 `json:"code"`
	ModelVersion   string                 `json:"version,omitempty"`
	ModelTitle     string                 `json:"title,omitempty"`
}

func (r ModelRef) Kind() modelcatalog.Kind           { return r.ModelKind }
func (r ModelRef) SubKind() modelcatalog.SubKind     { return r.ModelSubKind }
func (r ModelRef) Algorithm() modelcatalog.Algorithm { return r.ModelAlgorithm }
func (r ModelRef) Code() meta.Code                   { return meta.NewCode(r.ModelCode) }
func (r ModelRef) Version() string                   { return r.ModelVersion }
func (r ModelRef) Title() string                     { return r.ModelTitle }

func (r ModelRef) IsEmpty() bool { return r.ModelKind == "" && r.ModelCode == "" }

func (r ModelRef) IsScale() bool { return r.ModelKind == modelcatalog.KindScale }

func (r ModelRef) ExecutionIdentity() evalpipeline.ExecutionIdentity {
	return evalpipeline.ExecutionIdentity{Kind: r.ModelKind, SubKind: r.ModelSubKind, Algorithm: r.ModelAlgorithm}
}

func (r ModelRef) SameIdentity(other ModelRef) bool {
	return r.ExecutionIdentity() == other.ExecutionIdentity() && r.ModelCode == other.ModelCode && r.ModelVersion == other.ModelVersion
}

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

const (
	DimensionKindFactor  DimensionKind = "factor"
	DimensionKindPole    DimensionKind = "pole"
	DimensionKindTrait   DimensionKind = "trait"
	DimensionKindIndex   DimensionKind = "index"
	DimensionKindAbility DimensionKind = "ability"
)

type ScoreKind string

const (
	ScoreKindRawTotal     ScoreKind = "raw_total"
	ScoreKindMatchPercent ScoreKind = "match_percent"
	ScoreKindTScore       ScoreKind = "t_score"
	ScoreKindPercentile   ScoreKind = "percentile"
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

type ProfileKind string

const (
	ProfileKindPersonalityType  ProfileKind = "personality_type"
	ProfileKindPersonalityTrait ProfileKind = "personality_trait"
	ProfileKindAbilityProfile   ProfileKind = "ability_profile"
)

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
	// Typology classification facts. Display prose remains in the frozen
	// ReportInput attached to the durable Outcome record.
	Preference string
	Strength   *float64
	LeftPole   string
	RightPole  string
	Model      string
}

type ValidityResult struct {
	Code    string
	Label   string
	Passed  bool
	Message string
}

// NewExecution constructs the canonical in-memory evaluation result.
func NewExecution(
	modelRef ModelRef,
	summary Summary,
	detail Detail,
) *Execution {
	if detail.Kind == "" {
		detail.Kind = modelRef.Kind()
	}
	return &Execution{ModelRef: modelRef, Summary: summary, Detail: detail, Dimensions: make([]DimensionResult, 0), Validity: make([]ValidityResult, 0)}
}
