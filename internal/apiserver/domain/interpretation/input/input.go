// Package input defines the frozen, Interpretation-owned facts consumed by
// report builders. It contains no Assessment aggregate, evaluator, repository
// or report lifecycle state.
package input

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// InterpretationInput is the complete read-only input for one interpretation
// attempt. Association is correlation data only; it grants no Assessment write
// authority.
type InterpretationInput struct {
	OutcomeID           meta.ID
	Association         report.Association
	Model               report.ModelIdentity
	Runtime             RuntimeIdentity
	Result              ResultFacts
	Report              ReportSpec
	PresentationProfile *report.PresentationProfile
	FactorScoring       *FactorScoringFacts
	PersonalityType     *PersonalityTypeFacts
	TraitProfile        *TraitProfileFacts
}

type RuntimeIdentity struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	PayloadFormat   string
}

type ResultFacts struct {
	Primary *report.ScoreValue
	Level   *report.ResultLevel
}

// ReportSpec is the resolved, immutable report routing/template selection.
// TemplateVersion is deliberately explicit so Generation can use the same
// version in its idempotency key in the next batch.
type ReportSpec struct {
	ReportType      policy.ReportType
	TemplateVersion policy.TemplateVersion
	Algorithm       modelcatalog.Algorithm
	ProductChannel  modelcatalog.ProductChannel
	ReportProfile   policy.ReportProfile
	AdapterKey      string
	TemplateID      string
}

type FactorScoringFacts struct {
	Model   *reportscore.ReportModel
	Factors []reportscore.FactorReportScore
}

type PersonalityTypeFacts struct {
	Detail reporttypology.PersonalityTypeReportDetail
}

type TraitProfileFacts struct {
	Detail reporttypology.TraitProfileReportDetail
}
