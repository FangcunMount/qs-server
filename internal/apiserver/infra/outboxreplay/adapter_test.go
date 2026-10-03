package outboxreplay

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type ledgerStub struct {
	authorize func(context.Context, request.ReplayRequest) ([]request.ReplayResult, error)
	resolve   func(context.Context, request.ReplayRequest) ([]request.ReplayResult, bool, error)
}

func (l ledgerStub) Authorize(ctx context.Context, r request.ReplayRequest) ([]request.ReplayResult, error) {
	return l.authorize(ctx, r)
}
func (l ledgerStub) Resolve(ctx context.Context, r request.ReplayRequest) ([]request.ReplayResult, bool, error) {
	return l.resolve(ctx, r)
}

func replayInput() app.ReplayPendingInput {
	return app.ReplayPendingInput{Store: "profile", Reason: " approved\nreason ", Targets: []outboxport.ManualReplayTarget{{EventID: "second", ExpectedAttemptCount: 4}, {EventID: "first", ExpectedAttemptCount: 2}}}
}
func expectedRequest() request.ReplayRequest {
	input := replayInput()
	return request.ReplayRequest{OrgID: 7, RequestID: "request", Store: input.Store, Reason: input.Reason, Targets: []request.ReplayTarget{{EventID: "second", ExpectedFailureCount: 4}, {EventID: "first", ExpectedFailureCount: 2}}}
}
func durableResults() []request.ReplayResult {
	return []request.ReplayResult{{EventID: "second", Authorized: true}, {EventID: "first", Reason: "attempt_conflict"}}
}
func expectedResults() []outboxport.ManualReplayResult {
	return []outboxport.ManualReplayResult{{EventID: "second", Authorized: true}, {EventID: "first", Reason: "attempt_conflict"}}
}

func TestAuthorizePreservesCompleteInputAndOrderedResults(t *testing.T) {
	calls := 0
	ledger := ledgerStub{authorize: func(_ context.Context, r request.ReplayRequest) ([]request.ReplayResult, error) {
		calls++
		if !reflect.DeepEqual(r, expectedRequest()) {
			t.Fatalf("request=%+v", r)
		}
		return durableResults(), nil
	}, resolve: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, bool, error) {
		t.Fatal("successful authorization was reconciled")
		return nil, false, nil
	}}
	input := replayInput()
	result, err := AuthorizeManualReplayWithReason(context.Background(), ledger, input.Store, 7, "request", input.Reason, input.Targets)
	if err != nil || calls != 1 || !reflect.DeepEqual(result, expectedResults()) {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
	}
}

func TestAuthorizeRejectsInvalidInputBeforeLedger(t *testing.T) {
	cases := []struct {
		name string
		edit func(*app.ReplayPendingInput)
	}{
		{"zero failure count", func(i *app.ReplayPendingInput) { i.Targets[0].ExpectedAttemptCount = 0 }},
		{"negative failure count", func(i *app.ReplayPendingInput) { i.Targets[0].ExpectedAttemptCount = -1 }},
		{"duplicate event", func(i *app.ReplayPendingInput) { i.Targets[1].EventID = i.Targets[0].EventID }},
		{"blank reason", func(i *app.ReplayPendingInput) { i.Reason = " " }},
		{"empty targets", func(i *app.ReplayPendingInput) { i.Targets = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := replayInput()
			tc.edit(&input)
			ledger := ledgerStub{authorize: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, error) {
				t.Fatal("invalid request reached ledger")
				return nil, nil
			}}
			result, err := AuthorizeManualReplayWithReason(context.Background(), ledger, input.Store, 7, "request", input.Reason, input.Targets)
			if result != nil || !baseerrors.IsCode(err, code.ErrInvalidArgument) {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestInputConflictIsFinalAndDoesNotReconcile(t *testing.T) {
	ledger := ledgerStub{authorize: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, error) {
		return nil, fmt.Errorf("ledger: %w", request.ErrReplayInputConflict)
	},
		resolve: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, bool, error) {
			t.Fatal("conflict must not be reconciled")
			return nil, false, nil
		}}
	input := replayInput()
	result, err := AuthorizeManualReplayWithReason(context.Background(), ledger, input.Store, 7, "request", input.Reason, input.Targets)
	if result != nil || !baseerrors.IsCode(err, code.ErrConflict) || errors.Is(err, outboxport.ErrManualReplayOutcomeUnknown) {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

type contextKey struct{}

func TestUnknownAuthorizationReconcilesOnceAfterCancellation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		found      bool
		resolveErr error
	}{
		{"lost confirmation", true, nil},
		{"no durable result", false, nil},
		{"read failed", false, errors.New("read failed")},
		{"corrupt durable result", true, request.ErrReplayLedgerCorrupt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A caller's expired deadline must not suppress the read-only recovery.
			ctx, cancel := context.WithDeadline(context.WithValue(context.Background(), contextKey{}, "trace"), time.Now().Add(-time.Minute))
			defer cancel()
			authorizations, resolutions := 0, 0
			failure := errors.New("commit acknowledgement lost")
			ledger := ledgerStub{authorize: func(ctx context.Context, r request.ReplayRequest) ([]request.ReplayResult, error) {
				authorizations++
				if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
					t.Fatal("original context was replaced")
				}
				return nil, failure
			}, resolve: func(ctx context.Context, r request.ReplayRequest) ([]request.ReplayResult, bool, error) {
				resolutions++
				if ctx.Err() != nil || ctx.Value(contextKey{}) != "trace" {
					t.Fatalf("recovery context lost request value or cancellation independence: %v", ctx.Err())
				}
				deadline, ok := ctx.Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining <= 0 || remaining > 3*time.Second {
					t.Fatalf("unbounded recovery: %v %v", ok, remaining)
				}
				if !reflect.DeepEqual(r, expectedRequest()) {
					t.Fatalf("recovery changed request identity: %+v", r)
				}
				return durableResults(), tc.found, tc.resolveErr
			}}
			input := replayInput()
			result, err := AuthorizeManualReplayWithReason(ctx, ledger, input.Store, 7, "request", input.Reason, input.Targets)
			if authorizations != 1 || resolutions != 1 {
				t.Fatalf("authorization was retried: writes=%d reads=%d", authorizations, resolutions)
			}
			if tc.found && tc.resolveErr == nil {
				if err != nil || !reflect.DeepEqual(result, expectedResults()) {
					t.Fatalf("committed result lost: %+v %v", result, err)
				}
			} else if result != nil || !errors.Is(err, outboxport.ErrManualReplayOutcomeUnknown) || !strings.Contains(err.Error(), failure.Error()) || (tc.resolveErr != nil && !strings.Contains(err.Error(), tc.resolveErr.Error())) {
				t.Fatalf("uncertain result was finalized: %+v %v", result, err)
			}
		})
	}
}

func TestResolvePendingIsReadOnlyAndPreservesFoundAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		found bool
		err   error
	}{
		{"found", true, nil}, {"pending", false, nil}, {"corrupt", true, request.ErrReplayLedgerCorrupt}, {"failed", false, errors.New("lookup failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			ledger := ledgerStub{authorize: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, error) {
				t.Fatal("pending reconciliation wrote authorization")
				return nil, nil
			},
				resolve: func(_ context.Context, r request.ReplayRequest) ([]request.ReplayResult, bool, error) {
					calls++
					if !reflect.DeepEqual(r, expectedRequest()) {
						t.Fatalf("request=%+v", r)
					}
					return durableResults(), tc.found, tc.err
				}}
			result, found, err := ResolvePending(context.Background(), ledger, app.ActionAuditRecord{ActionID: "events.replay_pending", OrgID: 7, RequestID: "request"}, replayInput())
			if found != tc.found || err != tc.err || calls != 1 {
				t.Fatalf("found=%v err=%v calls=%d", found, err, calls)
			}
			if tc.found && tc.err == nil {
				if !reflect.DeepEqual(result, expectedResults()) {
					t.Fatalf("result=%+v", result)
				}
			} else if result != nil {
				t.Fatalf("unresolved items escaped: %+v", result)
			}
		})
	}
}

func TestResolvePendingRejectsWrongActionAndInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name, action string
		invalid      bool
	}{{"wrong action", "events.other", false}, {"invalid request", "events.replay_pending", true}} {
		t.Run(tc.name, func(t *testing.T) {
			input := replayInput()
			if tc.invalid {
				input.Targets[0].ExpectedAttemptCount = 0
			}
			ledger := ledgerStub{resolve: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, bool, error) {
				t.Fatal("invalid pending request reached ledger")
				return nil, false, nil
			}}
			result, found, err := ResolvePending(context.Background(), ledger, app.ActionAuditRecord{ActionID: tc.action, OrgID: 7, RequestID: "request"}, input)
			if err == nil || found || result != nil {
				t.Fatalf("result=%+v found=%v err=%v", result, found, err)
			}
			if baseerrors.IsCode(err, code.ErrInvalidArgument) {
				t.Fatalf("pending raw validation error changed: %v", err)
			}
		})
	}
}

func TestEmptyDurableResultsRemainNonNil(t *testing.T) {
	ledger := ledgerStub{authorize: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, error) { return nil, nil }, resolve: func(context.Context, request.ReplayRequest) ([]request.ReplayResult, bool, error) {
		return nil, true, nil
	}}
	input := replayInput()
	authorized, err := AuthorizeManualReplayWithReason(context.Background(), ledger, input.Store, 7, "request", input.Reason, input.Targets)
	pending, found, resolveErr := ResolvePending(context.Background(), ledger, app.ActionAuditRecord{ActionID: "events.replay_pending", OrgID: 7, RequestID: "request"}, input)
	if err != nil || resolveErr != nil || !found || authorized == nil || pending == nil || len(authorized) != 0 || len(pending) != 0 {
		t.Fatalf("nil/empty result shape changed: %v %v %v %v", authorized, pending, err, resolveErr)
	}
}
