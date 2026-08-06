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
			ReportType: policy.ReportTypeStandard,
			Algorithm:  modelcatalog.Algorithm(model.Algorithm),
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
		route, err := domainreporttemplate.ResolveFromAssets(materialized)
		if err != nil {
			return interpinput.InterpretationInput{}, classify(admission.KindArtifactContractInvalid, err, "resolve frozen report template route")
		}
		in.Report.TemplateID = route.TemplateID
		in.Report.TemplateVersion = route.TemplateVersion
	} else {
		return interpinput.InterpretationInput{}, classify(admission.KindArtifactContractInvalid, nil, "frozen interpretation assets are required")
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
		if routing.TemplateID != in.Report.TemplateID {
			return interpinput.InterpretationInput{}, classify(admission.KindArtifactContractInvalid, nil, "typology template id conflicts with frozen report sections")
		}
		in.Report.AdapterKey = string(routing.AdapterKey)
		version := policy.TemplateVersion(routing.TemplateVersion)
		if version.IsEmpty() {
			return interpinput.InterpretationInput{}, classify(admission.KindArtifactContractInvalid, nil, "typology template version is required")
		}
		if version != in.Report.TemplateVersion {
			return interpinput.InterpretationInput{}, classify(admission.KindArtifactContractInvalid, nil, "typology template version conflicts with frozen report sections")
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
