// Package outcome owns the immutable fact produced by a successful evaluation run.
package outcome

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const CurrentSchemaVersion uint = 2

type ID = meta.ID

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

// Record is the canonical, immutable output of one successful EvaluationRun.
// Payload keeps the versioned Execution JSON without forcing storage
// adapters to understand mechanism-specific detail DTOs.
type Record struct {
	id                     ID
	orgID                  int64
	assessmentID           meta.ID
	testeeID               uint64
	runID                  string
	model                  ModelIdentity
	runtime                RuntimeIdentity
	inputSnapshotRef       string
	reportInput            json.RawMessage
	payload                json.RawMessage
	schemaVersion          uint
	evaluatedAt            time.Time
	committedEventEvidence *eventevidence.EventEvidenceV1
}

type NewRecordInput struct {
	ID                     ID
	OrgID                  int64
	AssessmentID           meta.ID
	TesteeID               uint64
	RunID                  string
	Model                  ModelIdentity
	Runtime                RuntimeIdentity
	InputSnapshotRef       string
	ReportInput            json.RawMessage
	Payload                json.RawMessage
	SchemaVersion          uint
	EvaluatedAt            time.Time
	CommittedEventEvidence *eventevidence.EventEvidenceV1
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
	if input.CommittedEventEvidence != nil {
		if err := input.CommittedEventEvidence.Validate(); err != nil {
			return nil, err
		}
		if input.CommittedEventEvidence.BusinessBindingSHA256 != BusinessBindingSHA256(input) {
			return nil, fmt.Errorf("evaluation outcome event evidence business binding conflict")
		}
		if ref := input.CommittedEventEvidence.Reference; ref != nil && (ref.EventType != eventcatalog.EvaluationOutcomeCommitted || ref.Scope != fmt.Sprintf("org:%d", input.OrgID)) {
			return nil, fmt.Errorf("evaluation outcome event evidence identity conflict")
		}
	}
	input.EvaluatedAt = input.EvaluatedAt.UTC().Truncate(time.Millisecond)
	return &Record{
		id:                     input.ID,
		orgID:                  input.OrgID,
		assessmentID:           input.AssessmentID,
		testeeID:               input.TesteeID,
		runID:                  input.RunID,
		model:                  input.Model,
		runtime:                input.Runtime,
		inputSnapshotRef:       input.InputSnapshotRef,
		reportInput:            append(json.RawMessage(nil), input.ReportInput...),
		payload:                append(json.RawMessage(nil), input.Payload...),
		schemaVersion:          input.SchemaVersion,
		evaluatedAt:            input.EvaluatedAt,
		committedEventEvidence: input.CommittedEventEvidence.Clone(),
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

func (r *Record) CommittedEventEvidence() *eventevidence.EventEvidenceV1 {
	return r.committedEventEvidence.Clone()
}
func (r *Record) CommittedEventID() string {
	if r.committedEventEvidence == nil {
		return ""
	}
	return r.committedEventEvidence.EventID
}
func (r *Record) BusinessBindingSHA256() string {
	return BusinessBindingSHA256(NewRecordInput{ID: r.id, OrgID: r.orgID, AssessmentID: r.assessmentID, TesteeID: r.testeeID, RunID: r.runID, Model: r.model, Runtime: r.runtime, InputSnapshotRef: r.inputSnapshotRef, ReportInput: r.reportInput, Payload: r.payload, SchemaVersion: r.schemaVersion, EvaluatedAt: r.evaluatedAt})
}

// BusinessBindingSHA256 binds frozen identity, routing and original immutable
// result/input bytes. Technical metadata and mutable delivery state are excluded.
func BusinessBindingSHA256(in NewRecordInput) string {
	version := in.SchemaVersion
	if version == 0 {
		version = CurrentSchemaVersion
	}
	values := []string{strconv.FormatInt(in.OrgID, 10), in.AssessmentID.String(), strconv.FormatUint(in.TesteeID, 10), in.ID.String(), in.RunID,
		string(in.Model.Kind), string(in.Model.Algorithm), in.Model.Code, in.Model.Version, in.Model.Title, string(in.Runtime.DecisionKind), in.InputSnapshotRef,
		eventevidence.SourceDigest("outcome-payload", in.Payload).SHA256, eventevidence.SourceDigest("report-input", in.ReportInput).SHA256,
		strconv.FormatUint(uint64(version), 10), eventevidence.MillisecondTime(in.EvaluatedAt)}
	fields := make([]*string, 0, len(values))
	for _, value := range values {
		fields = append(fields, eventevidence.String(value))
	}
	return eventevidence.BindingDigest(eventcatalog.EvaluationOutcomeCommitted, fields...)
}
