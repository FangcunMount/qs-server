package originaleffect

import (
	"encoding/json"
	"testing"
	"time"

	domainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/stretchr/testify/require"
)

func ptr[T any](value T) *T { return &value }

func retryFixture() (evaluation.AssessmentPO, checkpoint.RuntimeCheckpointPO, payload.EvaluationRequestedData, time.Time) {
	now := time.Date(2026, 10, 3, 15, 0, 0, 123000000, time.FixedZone("UTC+8", 8*3600))
	var a evaluation.AssessmentPO
	a.ID, a.OrgID, a.TesteeID, a.AnswerSheetID = meta.FromUint64(71), 3, 29, 27
	a.Status, a.QuestionnaireCode, a.QuestionnaireVersion = "failed", "Q1", "v1"
	a.EvaluationModelKind, a.EvaluationModelAlgorithm, a.EvaluationModelCode, a.EvaluationModelVersion = ptr("scale"), ptr("factor_scoring"), ptr("M1"), ptr("v1")
	run := checkpoint.RuntimeCheckpointPO{Scope: "evaluation_run", ResourceID: "71:1", AttemptNo: 1, AssessmentID: ptr(uint64(71)), Status: "failed", FinishedAt: &now, Retryable: true, RetryDisposition: ptr("automatic"), RetryEventID: ptr("eval-retry:71:1:automatic"), NextAttemptAt: &now}
	d := payload.EvaluationRequestedData{OrgID: 3, AssessmentID: 71, TesteeID: 29, AnswerSheetID: "27", QuestionnaireCode: "Q1", QuestionnaireVer: "v1", ModelKind: "scale", ModelAlgorithm: "factor_scoring", ModelCode: "M1", ModelVersion: "v1", RequestedAt: now, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt"}
	return a, run, d, now
}

func TestOriginalRetryRequiresExactUnconsumedDueAuthority(t *testing.T) {
	a, run, data, now := retryFixture()
	for _, origin := range []string{"automatic", "manual", "force"} {
		d, r := data, run
		d.AttemptOrigin = origin
		if origin != "automatic" {
			d.ActionRequestID = "original-action"
			r.ActionRequestID = ptr(d.ActionRequestID)
		}
		if origin == "force" {
			r.Retryable = false
		}
		require.True(t, validRetry(a, r, d, *r.RetryEventID, now), origin)
	}
	tests := map[string]func(*evaluation.AssessmentPO, *checkpoint.RuntimeCheckpointPO, *payload.EvaluationRequestedData){
		"later accepted Run": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.Status = "running"
		},
		"later failed attempt": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.AttemptNo = 2
		},
		"not due": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.NextAttemptAt = ptr(now.Add(time.Second))
		},
		"original grant missing": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.RetryEventID = nil
		},
		"different grant": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.RetryEventID = ptr("new-grant")
		},
		"unapproved manual": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.AttemptOrigin = "manual"
			d.ActionRequestID = "new-action"
		},
		"unknown origin": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.AttemptOrigin = "lease_recovery"
		},
		"initial mode": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.Mode = "same_attempt"
		},
		"foreign org": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.OrgID = 9
		},
		"foreign testee": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.TesteeID = 30
		},
		"different answers": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.AnswerSheetID = "28"
		},
		"new model version": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.ModelVersion = "v2"
		},
		"new questionnaire version": func(_ *evaluation.AssessmentPO, _ *checkpoint.RuntimeCheckpointPO, d *payload.EvaluationRequestedData) {
			d.QuestionnaireVer = "v2"
		},
		"active lease": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.LeaseExpiresAt = ptr(now.Add(time.Minute))
		},
		"no classified completion": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.FinishedAt = nil
		},
		"manual disposition": func(_ *evaluation.AssessmentPO, r *checkpoint.RuntimeCheckpointPO, _ *payload.EvaluationRequestedData) {
			r.RetryDisposition = ptr("manual_required")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			a, r, d, at := retryFixture()
			mutate(&a, &r, &d)
			require.False(t, validRetry(a, r, d, "eval-retry:71:1:automatic", at))
		})
	}
}

func TestOriginalIntentChecksWireAndConfirmation(t *testing.T) {
	_, run, d, now := retryFixture()
	evt := domainevent.NewRetryRequestedEvent(domainevent.RequestedInput{OrgID: d.OrgID, AssessmentID: d.AssessmentID, TesteeID: d.TesteeID, EventID: *run.RetryEventID, RequestedAt: now, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt"})
	body, err := standardoutbox.EncodeWire(evt, "qs-apiserver")
	require.NoError(t, err)
	input := message.Input{Producer: "qs-server", ID: evt.EventID(), Destination: "qs.evaluation.lifecycle", EventType: EvaluationRetry, Scope: "org:3", SchemaVersion: "v1", ContentType: "application/json", OccurredAt: now.Format(time.RFC3339Nano), Payload: body}
	original, err := message.New(input)
	require.NoError(t, err)
	hash := original.Fingerprint()
	base := intentRow{Producer: input.Producer, MessageID: input.ID, Destination: input.Destination, EventType: input.EventType, Scope: input.Scope, SchemaVersion: input.SchemaVersion, ContentType: input.ContentType, OccurredAt: input.OccurredAt, Payload: body, Fingerprint: hash[:], State: "published", Version: 2, AttemptCount: 1, NextAttemptAt: now, TransportConfirmedAt: &now}
	request := Request{EventID: evt.EventID(), AssessmentID: 71, OrgID: 3, AcceptedBefore: now}
	got, _, err := validateIntent(base, request, now)
	require.NoError(t, err)
	require.Equal(t, original.Fingerprint(), got.Fingerprint())
	for name, mutate := range map[string]func(*intentRow){
		"unconfirmed":         func(r *intentRow) { r.TransportConfirmedAt = nil },
		"other scope":         func(r *intentRow) { r.Scope = "org:4" },
		"pending":             func(r *intentRow) { r.State = "pending" },
		"unattempted":         func(r *intentRow) { r.AttemptCount = 0 },
		"changed fingerprint": func(r *intentRow) { r.Fingerprint = make([]byte, 32) },
	} {
		t.Run(name, func(t *testing.T) {
			row := base
			mutate(&row)
			_, _, err := validateIntent(row, request, now)
			require.Error(t, err)
		})
	}
	// Even a self-consistent SDK fingerprint cannot override wire association.
	for _, field := range []string{"aggregate_id", "occurred_at", "source"} {
		t.Run(field, func(t *testing.T) {
			wire, _, err := legacy.Decode(base.Payload)
			require.NoError(t, err)
			wire.Metadata[field] = ""
			encoded, err := legacy.Encode(wire, legacy.Revision2)
			require.NoError(t, err)
			row := base
			row.Payload = encoded
			in := input
			in.Payload = encoded
			changed, err := message.New(in)
			require.NoError(t, err)
			fingerprint := changed.Fingerprint()
			row.Fingerprint = fingerprint[:]
			_, _, err = validateIntent(row, request, now)
			require.Error(t, err)
		})
	}
	// Reviewed cutoff is independent from transport confirmation.
	request.AcceptedBefore = now.Add(-time.Microsecond)
	_, _, err = validateIntent(base, request, now)
	require.Error(t, err)
}

func TestInitialReportRequiresCommittedFrozenAssociation(t *testing.T) {
	a, run, _, now := retryFixture()
	a.Status, run.Status = "evaluated", "succeeded"
	run.InputSnapshotRef = ptr("snapshot-original")
	o := evaluation.EvaluationOutcomePO{ID: 91, OrgID: 3, AssessmentID: 71, TesteeID: 29, EvaluationRunID: "71:1", ModelKind: "scale", ModelAlgorithm: ptr("factor_scoring"), ModelCode: "M1", ModelVersion: "v1", InputSnapshotRef: run.InputSnapshotRef, PayloadJSON: `{}`, ReportInputJSON: ptr(`{}`), SchemaVersion: 1, EvaluatedAt: now}
	d := payload.EvaluationOutcomeCommittedData{OrgID: 3, AssessmentID: 71, TesteeID: 29, EvaluationRunID: "71:1", OutcomeID: "91", CommittedAt: now}
	request := Request{AssessmentID: 71, OrgID: 3, AcceptedBefore: now}
	require.True(t, validInitial(a, run, o, d, request)) // Association only; Capture also uses the normal frozen adapter.
	for name, mutate := range map[string]func(*evaluation.EvaluationOutcomePO){
		"different model":      func(o *evaluation.EvaluationOutcomePO) { o.ModelVersion = "v2" },
		"different algorithm":  func(o *evaluation.EvaluationOutcomePO) { o.ModelAlgorithm = ptr("another") },
		"different assessment": func(o *evaluation.EvaluationOutcomePO) { o.AssessmentID = 72 },
		"different run":        func(o *evaluation.EvaluationOutcomePO) { o.EvaluationRunID = "71:2" },
		"different snapshot":   func(o *evaluation.EvaluationOutcomePO) { o.InputSnapshotRef = ptr("live-snapshot") },
		"missing frozen input": func(o *evaluation.EvaluationOutcomePO) { o.ReportInputJSON = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed := o
			mutate(&changed)
			require.False(t, validInitial(a, run, changed, d, request))
		})
	}
	// Output plans contain identity and hashes, never frozen answers or wire.
	encoded, err := json.Marshal(Plan{EventID: "event", original: message.Message{}})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "payload")
}

func TestOriginalBusinessTimeUsesSQLMillisWithoutEarlyRetry(t *testing.T) {
	a, run, data, now := retryFixture()
	data.RequestedAt = now.Add(700 * time.Microsecond)
	require.False(t, validRetry(a, run, data, *run.RetryEventID, now))
	require.True(t, validRetry(a, run, data, *run.RetryEventID, now.Add(time.Second)))
	run.NextAttemptAt = ptr(data.RequestedAt.Round(time.Millisecond))
	require.True(t, validRetry(a, run, data, *run.RetryEventID, now.Add(time.Second)))
	run.NextAttemptAt = ptr(data.RequestedAt.Round(time.Millisecond).Add(time.Millisecond))
	require.False(t, validRetry(a, run, data, *run.RetryEventID, now.Add(time.Second)))
}
