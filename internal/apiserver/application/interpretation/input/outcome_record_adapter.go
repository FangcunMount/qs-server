package input

import (
	"fmt"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
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
		return interpinput.InterpretationInput{}, fmt.Errorf("evaluation outcome is required")
	}
	execution, err := evaloutcome.RestoreExecution(record)
	if err != nil {
		return interpinput.InterpretationInput{}, err
	}
	assets, err := evaloutcome.RestoreReportInput(record)
	if err != nil {
		return interpinput.InterpretationInput{}, err
	}
	model := modelIdentityFromRecord(record)
	in := interpinput.InterpretationInput{
		OutcomeID: record.ID(),
		Association: report.Association{
			OrgID: record.OrgID(), AssessmentID: record.AssessmentID(), TesteeID: record.TesteeID(),
		},
		Model: model,
		Runtime: interpinput.RuntimeIdentity{
			AlgorithmFamily: record.Runtime().AlgorithmFamily,
			DecisionKind:    record.Runtime().DecisionKind,
			PayloadFormat:   record.Runtime().PayloadFormat,
		},
		Result: interpinput.ResultFacts{Primary: primary(execution), Level: level(execution)},
		Report: interpinput.ReportSpec{
			ReportType: policy.ReportTypeStandard, TemplateVersion: DefaultTemplateVersion,
			Algorithm: modelcatalog.Algorithm(model.Algorithm), ProductChannel: modelcatalog.ProductChannel(model.ProductChannel), Audience: policy.AudienceParticipant,
		},
	}
	if in.Runtime.AlgorithmFamily == "" {
		in.Runtime.AlgorithmFamily, _ = modelcatalog.AlgorithmFamilyFromIdentity(modelcatalog.Kind(model.Kind), modelcatalog.SubKind(model.SubKind), modelcatalog.Algorithm(model.Algorithm))
	}
	if in.Runtime.DecisionKind == "" {
		in.Runtime.DecisionKind = defaultDecisionKind(in.Runtime.AlgorithmFamily)
	}
	in.Report.ReportProfile = policy.ReportProfileForDecisionKind(in.Runtime.DecisionKind)

	switch in.Runtime.AlgorithmFamily {
	case modelcatalog.AlgorithmFamilyFactorScoring, modelcatalog.AlgorithmFamilyFactorNorm, modelcatalog.AlgorithmFamilyTaskPerformance:
		assetModel := factorModel(assets, in.Runtime.AlgorithmFamily)
		in.FactorScoring = &interpinput.FactorScoringFacts{Model: assetModel, Factors: factorScores(execution, assetModel)}
	case modelcatalog.AlgorithmFamilyFactorClassification:
		if err := populateTypologyFacts(&in, execution); err != nil {
			return interpinput.InterpretationInput{}, err
		}
		if payload, ok := evaluationinput.TypologyPayload(assets); ok && payload != nil {
			if runtimeSpec, err := payload.ToRuntimeSpec(); err == nil {
				in.Report.TemplateID = runtimeSpec.Report.TemplateID
				in.Report.AdapterKey = string(runtimeSpec.Report.ResolvedAdapterKey(runtimeSpec.OutcomeMapping, runtimeSpec.Decision.Kind))
			}
		}
	}
	return in, nil
}

func modelIdentityFromRecord(record *domainoutcome.Record) report.ModelIdentity {
	model := record.Model()
	algorithm := model.Algorithm
	if algorithm == "" {
		switch model.Kind {
		case modelcatalog.KindScale:
			algorithm = modelcatalog.AlgorithmScaleDefault
		case modelcatalog.KindTypology:
			algorithm = modelcatalog.AlgorithmPersonalityTypology
		}
	}
	return report.ModelIdentity{
		Kind: string(model.Kind), SubKind: string(model.SubKind), Algorithm: string(algorithm), Code: model.Code, Version: model.Version, Title: model.Title,
		ProductChannel:  string(modelcatalog.DefaultProductChannelFor(model.Kind)),
		AlgorithmFamily: binding.AlgorithmFamilyStringFromIdentity(model.Kind, model.SubKind, algorithm),
	}
}
