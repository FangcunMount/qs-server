package outcome

import (
	"encoding/json"
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Restore reconstructs the interpretation input exclusively from the durable
// EvaluationOutcome. The synthetic Assessment is read-only compatibility data
// for existing report builders and must never be persisted by Interpretation.
func Restore(record *domainoutcome.Record) (Outcome, error) {
	execution, err := RestoreExecution(record)
	if err != nil {
		return Outcome{}, err
	}
	modelRef := AssessmentModelRefFromExecution(execution.ModelRef)
	a := assessment.Reconstruct(
		record.AssessmentID(),
		record.OrgID(),
		testee.NewID(record.TesteeID()),
		assessment.QuestionnaireRef{},
		assessment.AnswerSheetRef{},
		assessment.NewAdhocOrigin(),
		assessment.StatusEvaluated,
		nil, nil, nil, nil, nil, nil,
		&modelRef,
	)
	reportInput, err := RestoreReportInput(record)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Assessment: a,
		Input:      reportInput,
		Execution:  execution,
		RuntimeDescriptorKey: evalpipeline.RuntimeDescriptorKey{
			AlgorithmFamily: record.Runtime().AlgorithmFamily,
			DecisionKind:    record.Runtime().DecisionKind,
			PayloadFormat:   record.Runtime().PayloadFormat,
		},
	}, nil
}

// RestoreExecution reconstructs only the immutable Evaluation execution fact.
// It intentionally does not load report input or synthesize an Assessment, so
// score queries can read the fact without depending on report compatibility data.
func RestoreExecution(record *domainoutcome.Record) (*domainoutcome.Execution, error) {
	if record == nil {
		return nil, fmt.Errorf("evaluation outcome is required")
	}
	var execution domainoutcome.Execution
	if err := json.Unmarshal(record.Payload(), &execution); err != nil {
		return nil, fmt.Errorf("decode evaluation outcome %s: %w", record.ID(), err)
	}
	model := record.Model()
	execution.ModelRef = domainoutcome.ModelRef{
		ModelKind: model.Kind, ModelSubKind: model.SubKind, ModelAlgorithm: model.Algorithm,
		ModelCode: model.Code, ModelVersion: model.Version, ModelTitle: model.Title,
	}
	if err := restoreTypedDetail(record, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

// RestoreReportInput reconstructs only the frozen report assets captured by a
// durable EvaluationOutcome. Interpretation may use this read-only helper to
// build its own input without reconstructing a synthetic Assessment.
func RestoreReportInput(record *domainoutcome.Record) (*evaluationinput.InputSnapshot, error) {
	if len(record.ReportInput()) == 0 {
		return nil, nil
	}
	var payload evaluationinput.ModelPayload
	switch record.Model().Kind {
	case modelcatalog.KindScale:
		var typed evaluationinput.ScaleModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, fmt.Errorf("decode report input %s: %w", record.ID(), err)
		}
		payload = typed
	case modelcatalog.KindTypology:
		var typed evaluationinput.TypologyModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, fmt.Errorf("decode report input %s: %w", record.ID(), err)
		}
		payload = typed
	case modelcatalog.KindBehavioralRating:
		var typed evaluationinput.BehavioralRatingModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, fmt.Errorf("decode report input %s: %w", record.ID(), err)
		}
		payload = typed
	case modelcatalog.KindCognitive:
		var typed evaluationinput.CognitiveModelPayload
		if err := json.Unmarshal(record.ReportInput(), &typed); err != nil {
			return nil, fmt.Errorf("decode report input %s: %w", record.ID(), err)
		}
		payload = typed
	default:
		return nil, fmt.Errorf("unsupported report input model kind %s", record.Model().Kind)
	}
	model := record.Model()
	snapshot := &evaluationinput.ModelSnapshot{
		Kind:      evaluationinput.EvaluationModelKind(model.Kind),
		SubKind:   string(model.SubKind),
		Algorithm: string(model.Algorithm),
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
		Payload:   payload,
	}
	return &evaluationinput.InputSnapshot{Model: snapshot, ModelPayload: payload}, nil
}

func restoreTypedDetail(record *domainoutcome.Record, execution *domainoutcome.Execution) error {
	var wire struct {
		Detail struct {
			Payload json.RawMessage
		}
	}
	if err := json.Unmarshal(record.Payload(), &wire); err != nil || len(wire.Detail.Payload) == 0 || string(wire.Detail.Payload) == "null" {
		return err
	}
	var target any
	switch record.Runtime().DecisionKind {
	case modelcatalog.DecisionKindPoleComposition, modelcatalog.DecisionKindNearestPattern:
		target = &outcometypology.PersonalityTypeDetail{}
	case modelcatalog.DecisionKindTraitProfile:
		target = &outcometypology.TraitProfileDetail{}
	default:
		target = &[]legacyFactorScoreWire{}
	}
	if err := json.Unmarshal(wire.Detail.Payload, target); err != nil {
		return fmt.Errorf("decode evaluation outcome detail %s: %w", record.ID(), err)
	}
	switch typed := target.(type) {
	case *outcometypology.PersonalityTypeDetail:
		execution.Detail.Payload = *typed
	case *outcometypology.TraitProfileDetail:
		execution.Detail.Payload = *typed
	case *[]legacyFactorScoreWire:
		// Schema v1 scale rows stored factor scores in Detail.Payload. Convert
		// that historical wire shape immediately into canonical dimensions so
		// no legacy scoring model escapes the persistence boundary.
		if len(execution.Dimensions) == 0 {
			execution.Dimensions = make([]domainoutcome.DimensionResult, 0, len(*typed))
			for _, factor := range *typed {
				dimension := domainoutcome.DimensionResult{
					Code: factor.FactorCode, Name: factor.FactorName,
					Kind:  domainoutcome.DimensionKindFactor,
					Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: factor.RawScore},
				}
				if factor.IsTotalScore {
					dimension.Role = "total"
				}
				if factor.RiskLevel != "" {
					dimension.Level = &domainoutcome.ResultLevel{Code: factor.RiskLevel, Label: factor.RiskLevel}
				}
				execution.Dimensions = append(execution.Dimensions, dimension)
			}
		}
		execution.Detail.Payload = nil
	}
	return nil
}

// legacyFactorScoreWire is a persistence-only decoder for schema v1 Outcome
// payloads. Conclusion and Suggestion are intentionally ignored: they are
// report prose, not Evaluation facts.
type legacyFactorScoreWire struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    string
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}
