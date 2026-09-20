package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type runtimeStoreStub struct {
	calls int
	org   int64
	items []RuntimeRequest
}

func (s *runtimeStoreStub) ListRuntime(_ context.Context, org int64, _ RuntimeQuery) (RuntimePage, error) {
	s.calls++
	s.org = org
	return RuntimePage{Items: append([]RuntimeRequest{}, s.items...)}, nil
}
func (s *runtimeStoreStub) GetRuntime(_ context.Context, org int64, _ string) (RuntimeRequest, error) {
	s.calls++
	s.org = org
	return s.items[0], nil
}

type runtimeGatewayStub struct {
	calls int
	ids   []string
	raw   json.RawMessage
	err   error
}

func (g *runtimeGatewayStub) ReadRuntime(_ context.Context, _ DraftScope, ids []string, _ bool) (json.RawMessage, error) {
	g.calls++
	g.ids = ids
	return g.raw, g.err
}
func TestRuntimeRetainsQSWhenAIFailsAndBatchesOnce(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	const sid = "00000000-0000-4000-8000-000000000002"
	s := &runtimeStoreStub{items: []RuntimeRequest{{RequestID: id}, {RequestID: id, SessionID: sid}}}
	g := &runtimeGatewayStub{err: errors.New("unavailable")}
	svc := RuntimeAdministration{Store: s, Gateway: g}
	scope := DraftScope{OrganizationID: 7, OperatorUserID: 42}
	p, e := svc.List(adminContext(), scope, RuntimeQuery{})
	if e != nil || !p.Partial || len(p.Items) != 2 || g.calls != 1 || len(g.ids) != 1 || s.org != 7 {
		t.Fatal(p, e, g)
	}
	g.err = nil
	g.raw = json.RawMessage(`{"observed_at":"2026-09-20T12:00:00Z","items":[{"request_id":"` + id + `","session_id":"` + sid + `","status":"running"}]}`)
	p, e = svc.List(adminContext(), scope, RuntimeQuery{})
	if e != nil || p.Partial || p.Items[1].AI == nil {
		t.Fatal(p, e)
	}
	g.err = ErrGovernanceDenied
	if _, e = svc.List(adminContext(), scope, RuntimeQuery{}); !errors.Is(e, ErrGovernanceDenied) {
		t.Fatal(e)
	}
	before := s.calls
	if _, e = svc.List(context.Background(), scope, RuntimeQuery{}); !errors.Is(e, ErrGovernanceDenied) || s.calls != before {
		t.Fatal(e)
	}
}
func TestRuntimeNoSessionStillVisibleWithoutRPC(t *testing.T) {
	s := &runtimeStoreStub{items: []RuntimeRequest{{RequestID: "00000000-0000-4000-8000-000000000001", CommandsPending: 1}}}
	g := &runtimeGatewayStub{}
	svc := RuntimeAdministration{Store: s, Gateway: g}
	r, e := svc.Get(adminContext(), DraftScope{OrganizationID: 7, OperatorUserID: 42}, s.items[0].RequestID)
	if e != nil || r.Partial || r.AIAvailability != "not_requested" || g.calls != 0 {
		t.Fatal(r, e)
	}
}
func TestRuntimeWindowAndBounds(t *testing.T) {
	now := time.Now()
	q := RuntimeQuery{}
	if e := q.Normalize(now); e != nil || q.Limit != 20 || q.Until.Sub(q.Since) != 7*24*time.Hour {
		t.Fatal(q, e)
	}
	for _, q := range []RuntimeQuery{{Limit: 51}, {AssessmentID: "0"}, {Status: "unknown"}, {History: true, Since: now}, {Since: now, Until: now}} {
		if e := q.Normalize(now); !errors.Is(e, ErrInvalid) {
			t.Fatal(q, e)
		}
	}
	q = RuntimeQuery{AssessmentID: "123"}
	if e := q.Normalize(now); e != nil || !q.Since.IsZero() {
		t.Fatal(q, e)
	}
}
