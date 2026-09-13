package aibridge

import (
	"context"
	"errors"
	"testing"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

func (g *publicationGatewayStub) ListPublicationHistory(_ context.Context, s PublicationScope, q PublicationHistoryQuery) (PublicationHistoryPage, error) {
	g.calls++
	g.scope = s
	return PublicationHistoryPage{Selector: q.Selector}, nil
}
func (g *publicationGatewayStub) GetPublicationHistory(_ context.Context, s PublicationScope, q PublicationSelector, version int64) (PublicationReceipt, error) {
	g.calls++
	g.scope = s
	return PublicationReceipt{Actor: "user:99", Current: PublicationState{Selector: q, Version: version}}, nil
}

func TestPublicationHistoryRequiresCurrentAuditPermission(t *testing.T) {
	g := &publicationGatewayStub{}
	s := &PublicationAdministration{Gateway: g}
	scope := PublicationScope{7, 42}
	q := PublicationHistoryQuery{Selector: publicationCommand().Expected.Selector, Limit: 20}
	audit := authz.WithSnapshot(context.Background(), &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:evaluation:collection:reports", Action: "audit", Mode: authz.AuthorizationModeUnconditional}}})
	if _, err := s.ListHistory(audit, scope, q); err != nil {
		t.Fatal(err)
	}
	r, err := s.GetHistory(audit, scope, q.Selector, 1)
	if err != nil || r.Actor != "user:99" || g.scope != scope || g.calls != 2 {
		t.Fatal(r, err, g)
	}
	if _, err := s.Disable(audit, scope, publicationCommand()); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal("audit granted write", err)
	}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if _, err := s.ListHistory(ctx, scope, q); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := s.GetHistory(ctx, scope, q.Selector, 1); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal(err)
		}
	}
	if _, err := s.ListHistory(audit, PublicationScope{7, 0}, q); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if g.calls != 2 {
		t.Fatal("denied read reached AI", g.calls)
	}
}

func TestPublicationHistoryRejectsInvalidQueriesBeforeGateway(t *testing.T) {
	g := &publicationGatewayStub{}
	s := &PublicationAdministration{Gateway: g}
	scope := PublicationScope{7, 42}
	base := PublicationHistoryQuery{Selector: publicationCommand().Expected.Selector, Limit: 20}
	for _, alter := range []func(*PublicationHistoryQuery){
		func(q *PublicationHistoryQuery) { q.Limit = 0 },
		func(q *PublicationHistoryQuery) { q.Limit = 21 },
		func(q *PublicationHistoryQuery) { q.BeforeVersion = -1 },
		func(q *PublicationHistoryQuery) { q.Selector.Audience = "unknown" },
	} {
		q := base
		alter(&q)
		if _, err := s.ListHistory(adminContext(), scope, q); !errors.Is(err, ErrInvalid) {
			t.Fatal(q, err)
		}
	}
	for _, version := range []int64{0, -1} {
		if _, err := s.GetHistory(adminContext(), scope, base.Selector, version); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if g.calls != 0 {
		t.Fatal("invalid query reached AI")
	}
}
