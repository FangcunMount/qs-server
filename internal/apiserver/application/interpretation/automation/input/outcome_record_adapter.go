package input

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationfactcodec "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// DefaultTemplateVersion freezes the current compatible interpretation assets
// until model-catalog publishes an explicit report-template version.
const DefaultTemplateVersion policy.TemplateVersion = policy.TemplateVersionV1

// FromOutcomeRecord builds the Interpretation-owned input directly from the
// immutable EvaluationOutcome. It intentionally does not reconstruct an
// Assessment or create application/evaluation/outcome.Outcome.
func FromOutcomeRecord(record *domainoutcome.Record) (interpinput.InterpretationInput, error) {
	if record == nil {
		return interpinput.InterpretationInput{}, classify(admission.KindOutcomeIncomplete, nil, "evaluation outcome is required")
	}
	execution, err := evaluationfactcodec.DecodeExecution(record)
	if err != nil {
		return interpinput.InterpretationInput{}, classify(admission.KindOutcomeIncomplete, err, "decode committed outcome")
	}
	assets, err := evaluationfactcodec.DecodeReportInput(record)
	if err != nil {
		return interpinput.InterpretationInput{}, classify(admission.KindArtifactContractInvalid, err, "decode frozen report input")
	}
	model := modelIdentityFromRecord(record)
	in := interpinput.InterpretationInput{
		OutcomeID: record.ID(),
		Association: report.Association{
			OrgID: record.OrgID(), AssessmentID: record.AssessmentID(), TesteeID: record.TesteeID(),
		},
		Model: model,
		Runtime: interpinput.RuntimeIdentity{
			DecisionKind: record.Runtime().DecisionKind,
		},
		Result: interpinput.ResultFacts{Primary: primary(execution), Level: level(execution)},
		Report: interpinput.ReportSpec{
			ReportType: policy.ReportTypeStandard, TemplateVersion: DefaultTemplateVersion,
			Algorithm: modelcatalog.Algorithm(model.Algorithm),
		},
	}
	family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(in.Runtime.DecisionKind)
	if !ok {
		return interpinput.InterpretationInput{}, classify(admission.KindCatalogIncompatible, nil, "evaluation outcome runtime identity is incomplete")
	}
	in.Report.ReportProfile = policy.ReportProfileForDecisionKind(in.Runtime.DecisionKind)
	if codes, ok := evaluationinput.FactorScoreVisibleCodesFromSnapshot(assets); ok {
		profile := report.NewFrozenPresentationProfile(codes)
		in.PresentationProfile = &profile
	}
	if materialized, ok := evaluationinput.InterpretationAssetsFromSnapshot(assets); ok {
		in.Report.TemplateVersion = domainreporttemplate.ResolveFromAssets(materialized)
	}

	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring, modelcatalog.AlgorithmFamilyFactorNorm, modelcatalog.AlgorithmFamilyTaskPerformance:
		assetModel := factorModel(assets, family)
		factors := factorScores(execution, assetModel)
		if err := applyFrozenNormInterpretation(factors, assets, in.PresentationProfile); err != nil {
			return interpinput.InterpretationInput{}, classify(admission.KindOutcomeAssociationMismatch, err, "validate frozen norm interpretation")
		}
		in.FactorScoring = &interpinput.FactorScoringFacts{Model: assetModel, Factors: factors}
	case modelcatalog.AlgorithmFamilyFactorClassification:
		if err := populateTypologyFacts(&in, execution, assets); err != nil {
			return interpinput.InterpretationInput{}, classify(admission.KindArtifactContractInvalid, err, "restore typology facts")
		}
		routing, ok := evaluationinput.TypologyReportRoutingFromSnapshot(assets)
		if !ok {
			return interpinput.InterpretationInput{}, classify(admission.KindCatalogIncompatible, nil, "report input typology routing is required")
		}
		in.Report.TemplateID = routing.TemplateID
		in.Report.AdapterKey = string(routing.AdapterKey)
		if version := policy.TemplateVersion(routing.TemplateVersion); !version.IsEmpty() {
			in.Report.TemplateVersion = version
		}
	}
	return in, nil
}

func modelIdentityFromRecord(record *domainoutcome.Record) report.ModelIdentity {
	model := record.Model()
	return report.ModelIdentity{
		Kind: string(model.Kind), Algorithm: string(model.Algorithm), Code: model.Code, Version: model.Version, Title: model.Title,
	}
}
