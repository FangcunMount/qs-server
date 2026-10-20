package retryschedule

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	outcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/stretchr/testify/require"
)

type routes struct{}

func (routes) GetTopicForEvent(string) (string, bool) { return LifecycleTopic, true }

func fixture(t *testing.T, kind string, origin retrygovernance.AttemptOrigin, request, id string, at time.Time) (Authority, message.Input) {
	t.Helper()
	a := Authority{Owner: "qs.mysql", OrgID: 7, Kind: kind, SubjectID: "41", RunID: "41:1", AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, Origin: origin, RequestID: request, DecisionHash: [32]byte{1}}
	base := event.BaseEvent{ID: id, EventTypeValue: catalog.EvaluationRetryRequested, OccurredAtValue: at, AggregateTypeValue: "Assessment", AggregateIDValue: a.SubjectID}
	var value event.DomainEvent
	if kind == Evaluation {
		value = event.Event[payload.EvaluationRequestedData]{BaseEvent: base, Data: payload.EvaluationRequestedData{OrgID: a.OrgID, AssessmentID: 41, TesteeID: a.TesteeID, ExpectedAttempt: a.ExpectedAttempt, AttemptOrigin: string(origin), ActionRequestID: request, Mode: "next_attempt", RequestedAt: at}}
	} else {
		a.Owner, a.SubjectID, a.RunID, a.OutcomeID = "qs.mongo", "51", "61", 71
		base.EventTypeValue, base.AggregateTypeValue, base.AggregateIDValue = catalog.InterpretationRetryRequested, "ReportGeneration", a.SubjectID
		value = event.Event[outcome.InterpretationRetryRequestedPayload]{BaseEvent: base, Data: outcome.InterpretationRetryRequestedPayload{OrgID: 7, GenerationID: "51", RunID: "61", AssessmentID: "41", OutcomeID: "71", TesteeID: 9, ExpectedAttempt: 1, AttemptOrigin: string(origin), ActionRequestID: request, Mode: "next_attempt", RequestedAt: at}}
	}
	prepared, err := standard.PrepareIntents([]event.DomainEvent{value}, routes{}, "api-server")
	require.NoError(t, err)
	return a, prepared[0].Message.Input()
}

func TestScheduleFreezesFirstWireWithoutChangingItsFutureDeadline(t *testing.T) {
	for _, kind := range []string{Evaluation, Interpretation} {
		t.Run(kind, func(t *testing.T) {
			at := time.Date(2026, 10, 20, 15, 30, 0, 123499999, time.UTC)
			a, in := fixture(t, kind, retrygovernance.AttemptOriginAutomatic, "", "first", at)
			wire := bytes.Clone(in.Payload)
			f, err := New(a, in, at)
			require.NoError(t, err)
			in.Payload[0] ^= 1
			copy := f.Message().Input()
			copy.Payload[0] ^= 1
			require.Equal(t, wire, f.Message().Input().Payload)
			require.True(t, f.DueAt().Equal(at))
			raw := f.Bytes()
			decoded, err := Decode(raw)
			require.NoError(t, err)
			require.Equal(t, raw, decoded.Bytes())
			require.Equal(t, f.ID(), decoded.ID())
			require.Equal(t, f.Digest(), decoded.Digest())
			raw[0] ^= 1
			require.NotEqual(t, raw, f.Bytes())
		})
	}
}

func TestScheduleReservationIdentitySeparatesBusinessInputFromWinnerArtifacts(t *testing.T) {
	at := time.Date(2026, 10, 20, 15, 30, 0, 0, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginManual, "request-1", "first", at)
	first, err := New(a, in, at)
	require.NoError(t, err)
	b, other := fixture(t, Evaluation, retrygovernance.AttemptOriginManual, "request-1", "candidate", at.Add(time.Minute))
	candidate, err := New(b, other, at.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, first.Key(), candidate.Key())
	require.Equal(t, first.InputHash(), candidate.InputHash())
	require.NoError(t, first.MatchInput(candidate))
	require.NotEqual(t, first.Digest(), candidate.Digest())
	b.DecisionHash = [32]byte{2}
	changed, err := New(b, other, at.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, first.Key(), changed.Key())
	require.ErrorIs(t, first.MatchInput(changed), ErrConflict)
	b, other = fixture(t, Evaluation, retrygovernance.AttemptOriginForce, "request-1", "forced", at)
	forced, err := New(b, other, at)
	require.NoError(t, err)
	require.Equal(t, first.Key(), forced.Key())
	require.ErrorIs(t, first.MatchInput(forced), ErrConflict)
	_, err = Decode(append(first.Bytes(), '\n'))
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Decode(bytes.Replace(first.Bytes(), []byte(`"Version":1`), []byte(`"Version":2`), 1))
	require.ErrorIs(t, err, ErrInvalid)
}

func TestScheduleRejectsCrossOwnerScopeRunAndEarlyDispatchIndex(t *testing.T) {
	at := time.Date(2026, 10, 20, 15, 30, 0, 999999999, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "first", at)
	for _, change := range []string{"owner", "org", "subject", "run", "attempt", "testee", "policy", "due"} {
		t.Run(change, func(t *testing.T) {
			b, due := a, at
			switch change {
			case "owner":
				b.Owner = "qs.mongo"
			case "org":
				b.OrgID++
			case "subject":
				b.SubjectID = "041"
			case "run":
				b.RunID = "42:1"
			case "attempt":
				b.ExpectedAttempt++
			case "testee":
				b.TesteeID++
			case "policy":
				b.DecisionHash = [32]byte{}
			case "due":
				due = at.Add(time.Second)
			}
			_, err := New(b, in, due)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
	for _, nanos := range []int{0, 123000000, 123000001, 999999999} {
		t.Run(fmt.Sprint(nanos), func(t *testing.T) {
			at := time.Date(2026, 10, 20, 15, 30, 0, nanos, time.UTC)
			a, in := fixture(t, Interpretation, retrygovernance.AttemptOriginAutomatic, "", "first", at)
			f, err := New(a, in, at)
			require.NoError(t, err)
			for _, precision := range []time.Duration{time.Microsecond, time.Millisecond} {
				index, err := f.IndexDue(precision)
				require.NoError(t, err)
				require.False(t, index.Before(at), "database precision cannot advance the authorized retry")
				require.Less(t, index.Sub(at), precision)
			}
		})
	}
}
