package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type decisionProjectionUpdater struct {
	called    bool
	onPersist func()
}

func (u *decisionProjectionUpdater) PersistFromSnapshot(context.Context, *operatorapp.OperatorResult, *authz.Snapshot) error {
	return nil
}

func (u *decisionProjectionUpdater) PersistFromSnapshotByUser(_ context.Context, _, _ int64, _ *authz.Snapshot) error {
	u.called = true
	if u.onPersist != nil {
		u.onPersist()
	}
	return nil
}

func (u *decisionProjectionUpdater) SyncRoles(context.Context, int64, uint64) error { return nil }

func TestGRPCDecisionRechecksSnapshotAfterRoleProjection(t *testing.T) {
	t.Parallel()
	for _, advanced := range []bool{false, true} {
		name := "current proof"
		if advanced {
			name = "committed version advanced during projection"
		}
		t.Run(name, func(t *testing.T) {
			snap := &authz.Snapshot{AuthzVersion: 7}
			committed := int64(7)
			updater := &decisionProjectionUpdater{onPersist: func() {
				if advanced {
					committed = 8
				}
			}}
			verified := false
			handlerCalled := false
			result, err := invokeWithAuthorizationSnapshot(
				context.Background(), "request", func(ctx context.Context, req interface{}) (interface{}, error) {
					handlerCalled = true
					got, ok := authz.FromContext(ctx)
					if !ok || got != snap || actorctx.GrantingUserID(ctx) != 701 || req != "request" {
						t.Fatalf("handler did not receive the checked snapshot and caller identity")
					}
					return "admitted", nil
				}, "701", 88,
				func(_ context.Context, userID string) (*authz.Snapshot, error) {
					if userID != "701" {
						t.Fatalf("loaded snapshot for wrong user: %s", userID)
					}
					return snap, nil
				},
				func(_ context.Context, got *authz.Snapshot) error {
					verified = true
					if !updater.called || got != snap {
						t.Fatal("decision check did not follow role projection")
					}
					if got.AuthzVersion < committed {
						return errors.New("committed version advanced")
					}
					return nil
				}, updater,
			)
			if !verified {
				t.Fatal("snapshot was not rechecked at handler admission")
			}
			if advanced {
				if status.Code(err) != codes.Unavailable || handlerCalled {
					t.Fatalf("stale gRPC snapshot entered handler: result=%v err=%v handler=%v", result, err, handlerCalled)
				}
				return
			}
			if err != nil || result != "admitted" || !handlerCalled {
				t.Fatalf("current gRPC snapshot was rejected: result=%v err=%v handler=%v", result, err, handlerCalled)
			}
		})
	}
}
