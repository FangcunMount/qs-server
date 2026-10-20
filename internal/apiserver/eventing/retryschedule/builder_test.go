package retryschedule

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/stretchr/testify/require"
)

func TestGovernedPlanFreezesAuthorizationAndFirstWireBeforeOriginalRoot(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 123499999, time.UTC)
	a, _ := fixture(t, Evaluation, retrygovernance.AttemptOriginManual, "operation-1", "first", at)
	evt := event.Event[payload.EvaluationRequestedData]{BaseEvent: event.BaseEvent{ID: "first", EventTypeValue: "evaluation.retry.requested", OccurredAtValue: at, AggregateTypeValue: "Evaluation", AggregateIDValue: "41"}, Data: payload.EvaluationRequestedData{OrgID: 7, AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, AttemptOrigin: "manual", ActionRequestID: "operation-1", Mode: "next_attempt", RequestedAt: at}}
	d := &retrygovernance.Decision{Disposition: retrygovernance.DispositionAutomatic, Attempt: 1, MaxAutomaticAttempts: 3, RemainingAutomaticAttempts: 2, NextAttemptAt: &at, PolicyVersion: "business-retry/v1", RetryEventID: "first", ActionRequestID: "operation-1"}
	command := map[string]string{"request_id": "operation-1", "subject": "user:9", "action": "retry", "reason": "approved recovery"}
	f, err := (Builder{Resolver: routes{}}).Freeze(a, evt, d, command)
	require.NoError(t, err)
	expected, err := json.Marshal(command)
	require.NoError(t, err)
	require.Equal(t, expected, f.Authority().Authorization)
	exposed := f.Authority()
	exposed.Authorization[0] ^= 1
	require.Equal(t, expected, f.Authority().Authorization)
	command["reason"] = "different authorization"
	changed, err := (Builder{Resolver: routes{}}).Freeze(a, evt, d, command)
	require.NoError(t, err)
	require.Equal(t, f.Key(), changed.Key())
	require.ErrorIs(t, f.MatchInput(changed), ErrConflict)
	_, err = (Builder{Resolver: routes{}}).Freeze(a, evt, d, nil)
	require.ErrorIs(t, err, ErrInvalid)
	var missing *struct{}
	_, err = (Builder{Resolver: routes{}}).Freeze(a, evt, d, missing)
	require.ErrorIs(t, err, ErrInvalid)
	// The command audit stays in the private plan, not the consumer's original wire.
	require.NotContains(t, string(f.Message().Input().Payload), "approved recovery")
}

func TestAutomaticPlanRetainsPreGovernedCodecBytes(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 123499999, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "first", at)
	current, err := New(a, in, at)
	require.NoError(t, err)
	// This exact field order is the already-validated558898741 authority codec.
	// Adding private governed audit fields must not rewrite automatic history.
	legacyAuthority := struct {
		Owner           string
		OrgID           int64
		Kind            string
		SubjectID       string
		RunID           string
		AssessmentID    uint64
		OutcomeID       uint64
		TesteeID        uint64
		ExpectedAttempt int
		Origin          retrygovernance.AttemptOrigin
		RequestID       string
		DecisionHash    [32]byte
	}{a.Owner, a.OrgID, a.Kind, a.SubjectID, a.RunID, a.AssessmentID, a.OutcomeID, a.TesteeID, a.ExpectedAttempt, a.Origin, a.RequestID, a.DecisionHash}
	legacy, err := json.Marshal(struct {
		Version   int
		Authority any
		DueAt     time.Time
		Message   message.Input
	}{1, legacyAuthority, at, in})
	require.NoError(t, err)
	require.Equal(t, legacy, current.Bytes())
	read, err := Decode(legacy)
	require.NoError(t, err)
	require.Equal(t, current.Digest(), read.Digest())
	require.Empty(t, read.Authority().Authorization)
}
