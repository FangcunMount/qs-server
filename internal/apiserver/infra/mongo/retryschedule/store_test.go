package retryschedule

import (
	"testing"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	outcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type testRoutes struct{}

func (testRoutes) GetTopicForEvent(string) (string, bool) { return model.LifecycleTopic, true }

func plan(t *testing.T) model.Frozen {
	t.Helper()
	at := time.Date(2026, 10, 21, 0, 0, 0, 123499999, time.UTC)
	a := model.Authority{Owner: "qs.mongo", OrgID: 7, Kind: model.Interpretation, SubjectID: "51", RunID: "61", AssessmentID: 41, OutcomeID: 71, TesteeID: 9, ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: [32]byte{1}}
	evt := event.Event[outcome.InterpretationRetryRequestedPayload]{BaseEvent: event.BaseEvent{ID: "retry-first", EventTypeValue: catalog.InterpretationRetryRequested, OccurredAtValue: at, AggregateTypeValue: "ReportGeneration", AggregateIDValue: "51"}, Data: outcome.InterpretationRetryRequestedPayload{OrgID: 7, GenerationID: "51", RunID: "61", AssessmentID: "41", OutcomeID: "71", TesteeID: 9, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: at}}
	prepared, err := standard.PrepareIntents([]event.DomainEvent{evt}, testRoutes{}, "api-server")
	require.NoError(t, err)
	f, err := model.New(a, prepared[0].Message.Input(), at)
	require.NoError(t, err)
	return f
}

func inertClient(t *testing.T) *mongo.Client {
	t.Helper()
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://127.0.0.1:1")) //nolint:staticcheck // Deliberately construct without Connect using the pinned v1.17.6 API.
	require.NoError(t, err)
	return client
}

type sessionStub struct {
	mongo.Session
	client *mongo.Client
}

func (s sessionStub) Client() *mongo.Client { return s.client }

func TestScheduleConstructorAndInvalidRootNeverConnectOrInstallSchema(t *testing.T) {
	client := inertClient(t)
	s, err := New(client.Database("original"), transaction.Binding{Owner: "qs.mongo", StoreID: "primary", WriterEpoch: "m3-1"})
	require.NoError(t, err)
	f := plan(t)
	_, err = s.SaveFrozen(t.Context(), f)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	foreign := mongo.NewSessionContext(t.Context(), sessionStub{client: inertClient(t)})
	_, err = s.SaveFrozen(foreign, f)
	require.ErrorIs(t, err, transaction.ErrBinding)
	ordinary := mongo.NewSessionContext(t.Context(), sessionStub{client: client})
	_, err = s.SaveFrozen(ordinary, f)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	_, err = s.Find(ordinary, f)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	_, err = s.Due(t.Context(), f.DueAt(), 101)
	require.ErrorIs(t, err, model.ErrInvalid)
	_, err = New(client.Database("original"), transaction.Binding{Owner: "qs.mysql", StoreID: "primary", WriterEpoch: "m3-1"})
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = New(client.Database(""), transaction.Binding{Owner: "qs.mongo", StoreID: "primary", WriterEpoch: "m3-1"})
	require.ErrorIs(t, err, transaction.ErrBinding)
	// No Connect was called and every path above rejected before an I/O
	// operation. This unit proof does not claim real-session atomicity.
}

func TestScheduleReaderRejectsCorruptOriginalIdentityAndPrematureDueIndex(t *testing.T) {
	for _, mode := range []string{"ready", "key", "scope", "hash", "early_index", "committed_without_proof"} {
		t.Run(mode, func(t *testing.T) {
			s, err := New(inertClient(t).Database("original"), transaction.Binding{Owner: "qs.mongo", StoreID: "primary", WriterEpoch: "m3-1"})
			require.NoError(t, err)
			f := plan(t)
			a, in := f.Authority(), f.Message().Input()
			key, input, digest := f.Key(), f.InputHash(), f.Digest()
			due, err := f.IndexDue(time.Millisecond)
			require.NoError(t, err)
			d := document{ID: key[:], Identity: f.KeyIdentity(), Input: input[:], Digest: digest[:], Owner: a.Owner, Scope: in.Scope, Kind: a.Kind, Subject: a.SubjectID, Run: a.RunID, Message: in.ID, Due: due, Frozen: f.Bytes(), State: model.Ready, Version: 1}
			switch mode {
			case "key":
				d.ID[0] ^= 1
			case "scope":
				d.Scope = "org:8"
			case "hash":
				d.Input[0] ^= 1
			case "early_index":
				d.Due = d.Due.Add(-time.Millisecond)
			case "committed_without_proof":
				d.State = model.DispatchCommitted
			}
			r, err := s.decode(d)
			if mode == "ready" {
				require.NoError(t, err)
				require.Equal(t, f.Bytes(), r.Plan.Bytes())
			} else {
				require.ErrorIs(t, err, model.ErrInvalid)
			}
		})
	}
}
