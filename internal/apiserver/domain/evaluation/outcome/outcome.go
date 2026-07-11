// Package outcome owns the immutable fact produced by a successful evaluation run.
package outcome

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const CurrentSchemaVersion uint = 1

type ID = meta.ID

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

// Record is the canonical, immutable output of one successful EvaluationRun.
// Payload keeps the versioned AssessmentOutcome JSON without forcing storage
// adapters to understand mechanism-specific detail DTOs.
type Record struct {
	id               ID
	orgID            int64
	assessmentID     meta.ID
	testeeID         uint64
	runID            string
	model            ModelIdentity
	runtime          RuntimeIdentity
	inputSnapshotRef string
	reportInput      json.RawMessage
	payload          json.RawMessage
	schemaVersion    uint
	evaluatedAt      time.Time
}

type NewRecordInput struct {
	ID               ID
	OrgID            int64
	AssessmentID     meta.ID
	TesteeID         uint64
	RunID            string
	Model            ModelIdentity
	Runtime          RuntimeIdentity
	InputSnapshotRef string
	ReportInput      json.RawMessage
	Payload          json.RawMessage
	SchemaVersion    uint
	EvaluatedAt      time.Time
}

func NewRecord(input NewRecordInput) (*Record, error) {
	if input.ID.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment id is required")
	}
	if input.TesteeID == 0 {
		return nil, fmt.Errorf("testee id is required")
	}
	if input.RunID == "" {
		return nil, fmt.Errorf("evaluation run id is required")
	}
	if input.Model.Kind == "" || input.Model.Code == "" {
		return nil, fmt.Errorf("evaluation model reference is required")
	}
	if len(input.Payload) == 0 {
		return nil, fmt.Errorf("evaluation outcome payload is required")
	}
	if input.SchemaVersion == 0 {
		input.SchemaVersion = CurrentSchemaVersion
	}
	if input.EvaluatedAt.IsZero() {
		return nil, fmt.Errorf("evaluated at is required")
	}
	return &Record{
		id:               input.ID,
		orgID:            input.OrgID,
		assessmentID:     input.AssessmentID,
		testeeID:         input.TesteeID,
		runID:            input.RunID,
		model:            input.Model,
		runtime:          input.Runtime,
		inputSnapshotRef: input.InputSnapshotRef,
		reportInput:      append(json.RawMessage(nil), input.ReportInput...),
		payload:          append(json.RawMessage(nil), input.Payload...),
		schemaVersion:    input.SchemaVersion,
		evaluatedAt:      input.EvaluatedAt,
	}, nil
}

func (r *Record) ID() ID { return r.id }

func (r *Record) OrgID() int64 { return r.orgID }

func (r *Record) AssessmentID() meta.ID { return r.assessmentID }

func (r *Record) TesteeID() uint64 { return r.testeeID }

func (r *Record) RunID() string { return r.runID }

func (r *Record) Model() ModelIdentity { return r.model }

func (r *Record) Runtime() RuntimeIdentity { return r.runtime }

func (r *Record) InputSnapshotRef() string { return r.inputSnapshotRef }

func (r *Record) ReportInput() json.RawMessage {
	return append(json.RawMessage(nil), r.reportInput...)
}

func (r *Record) Payload() json.RawMessage {
	return append(json.RawMessage(nil), r.payload...)
}

func (r *Record) SchemaVersion() uint { return r.schemaVersion }

func (r *Record) EvaluatedAt() time.Time { return r.evaluatedAt }
