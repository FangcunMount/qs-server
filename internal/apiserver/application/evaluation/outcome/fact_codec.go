package outcome

import (
	"encoding/json"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationfactcodec "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
)

// FactRecord copies an Evaluation-owned record into the immutable read contract.
func FactRecord(record *domainoutcome.Record) *evaluationfact.Record {
	if record == nil {
		return nil
	}
	model := record.Model()
	runtime := record.Runtime()
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: record.ID(), OrgID: record.OrgID(), AssessmentID: record.AssessmentID(), TesteeID: record.TesteeID(), RunID: record.RunID(),
		Model:            evaluationfact.ModelIdentity{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm, Code: model.Code, Version: model.Version, Title: model.Title},
		Runtime:          evaluationfact.RuntimeIdentity{AlgorithmFamily: runtime.AlgorithmFamily, DecisionKind: runtime.DecisionKind, PayloadFormat: runtime.PayloadFormat},
		InputSnapshotRef: record.InputSnapshotRef(), SchemaVersion: record.SchemaVersion(), Payload: record.Payload(), ReportInput: record.ReportInput(), EvaluatedAt: record.EvaluatedAt(),
	})
}

// RestoreExecution decodes a durable record through the shared versioned fact codec.
func RestoreExecution(record *domainoutcome.Record) (*domainoutcome.Execution, error) {
	decoded, err := evaluationfactcodec.DecodeExecution(FactRecord(record))
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	var execution domainoutcome.Execution
	if err := json.Unmarshal(payload, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}
