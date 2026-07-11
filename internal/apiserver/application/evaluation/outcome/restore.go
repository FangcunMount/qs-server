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
	modelRef := AssessmentOutcomeFromExecution(execution).ModelRef
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
	reportInput, err := restoreReportInput(record)
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

func restoreReportInput(record *domainoutcome.Record) (*evaluationinput.InputSnapshot, error) {
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
		target = &[]assessment.FactorScoreResult{}
	}
	if err := json.Unmarshal(wire.Detail.Payload, target); err != nil {
		return fmt.Errorf("decode evaluation outcome detail %s: %w", record.ID(), err)
	}
	switch typed := target.(type) {
	case *outcometypology.PersonalityTypeDetail:
		execution.Detail.Payload = *typed
	case *outcometypology.TraitProfileDetail:
		execution.Detail.Payload = *typed
	case *[]assessment.FactorScoreResult:
		execution.Detail.Payload = *typed
	}
	return nil
}
