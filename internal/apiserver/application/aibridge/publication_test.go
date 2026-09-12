package aibridge

import (
	"context"
	"errors"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"testing"
)

type publicationGatewayStub struct {
	PublicationGateway
	calls int
	scope PublicationScope
}

func (g *publicationGatewayStub) DisablePublication(_ context.Context, scope PublicationScope, c PublicationCommand) (PublicationReceipt, error) {
	g.calls++
	g.scope = scope
	return PublicationReceipt{CommandID: c.CommandID, Reason: c.Reason}, nil
}
func (g *publicationGatewayStub) GetPublication(_ context.Context, scope PublicationScope, selector PublicationSelector) (PublicationState, error) {
	g.calls++
	g.scope = scope
	return PublicationState{Selector: selector}, nil
}
func publicationCommand() PublicationCommand {
	v := int64(0)
	return PublicationCommand{CommandID: "00000000-0000-4000-8000-000000000001", Expected: &PublicationExpectation{Selector: PublicationSelector{Audience: "participant", ModelKind: "scale", DecisionKind: "score_range"}, Version: &v}, Reason: "  核对原命令  ", Confirm: true}
}
func TestPublicationRequiresFreshPermissionAndIdentity(t *testing.T) {
	g := &publicationGatewayStub{}
	s := &PublicationAdministration{Gateway: g}
	scope := PublicationScope{7, 42}
	command := publicationCommand()
	receipt, err := s.Disable(adminContext(), scope, command)
	if err != nil || g.calls != 1 || g.scope != scope || receipt.Reason != command.Reason {
		t.Fatal("original scoped command changed", err)
	}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if _, err = s.Disable(ctx, scope, command); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err = s.Get(ctx, scope, command.Expected.Selector); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal(err)
		}
	}
	if _, err = s.Disable(adminContext(), PublicationScope{7, 0}, command); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if g.calls != 1 {
		t.Fatal("denied request reached AI")
	}
}
func TestPublicationRequiresExplicitVersionZeroAndConfirmation(t *testing.T) {
	g := &publicationGatewayStub{}
	s := &PublicationAdministration{Gateway: g}
	for _, change := range []func(*PublicationCommand){func(c *PublicationCommand) { c.Expected = nil }, func(c *PublicationCommand) { c.Expected.Version = nil }, func(c *PublicationCommand) { v := int64(-1); c.Expected.Version = &v }, func(c *PublicationCommand) { c.Confirm = false }, func(c *PublicationCommand) { c.CommandID = "00000000-0000-0000-0000-000000000000" }, func(c *PublicationCommand) { c.Expected.ActivePublicationID = "wrong" }, func(c *PublicationCommand) { c.Reason = "<script>" }, func(c *PublicationCommand) { v := "v1"; c.Expected.Selector.ModelVersion = &v }} {
		c := publicationCommand()
		change(&c)
		if _, err := s.Disable(adminContext(), PublicationScope{7, 42}, c); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if g.calls != 0 {
		t.Fatal("invalid command forwarded")
	}
	if _, err := s.Disable(adminContext(), PublicationScope{7, 42}, publicationCommand()); err != nil {
		t.Fatal("explicit zero rejected", err)
	}
}
