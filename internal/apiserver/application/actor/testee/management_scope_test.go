package testee

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/stretchr/testify/require"
	"testing"
)

type updateScopeStub struct{ calls int }

func (s *updateScopeStub) ResolveStoreRange(_ context.Context, org, user int64, resource, action string) (authz.StoreRange, error) {
	s.calls++
	if org != 1 || user != 9 || resource != "qs:actor:collection:testees" || action != "update" {
		panic("wrong update authorization")
	}
	return authz.StoreRange{StoreIDs: []uint64{7}}, nil
}
func TestManagementUpdateRequiresOwnActionCompanyAndStore(t *testing.T) {
	access := &updateScopeStub{}
	service := &managementService{access: access}
	target := domain.NewTestee(1, "T", domain.Gender(0), nil)
	store := uint64(7)
	target.RestoreStore(&store, 1)
	ctx := actorctx.WithOperatorOrgID(actorctx.WithGrantingUserID(context.Background(), 9), 1)
	require.NoError(t, service.authorizeUpdate(ctx, target))
	store = 8
	target.RestoreStore(&store, 2)
	require.Error(t, service.authorizeUpdate(ctx, target))
	target.RestoreStore(nil, 3)
	require.Error(t, service.authorizeUpdate(ctx, target))
	calls := access.calls
	require.Error(t, service.authorizeUpdate(actorctx.WithOperatorOrgID(ctx, 2), target))
	require.Error(t, service.authorizeUpdate(context.Background(), target))
	require.Equal(t, calls, access.calls)
	require.NoError(t, (&managementService{selfService: true}).authorizeUpdate(context.Background(), target))
}

type lockedManagementRepo struct {
	assessmentAttentionRepoStub
	lockCalls int
}

func (r *lockedManagementRepo) FindByIDForUpdate(_ context.Context, org int64, id domain.ID) (*domain.Testee, error) {
	r.lockCalls++
	if org != 1 || id != 10 {
		panic("incorrect locked lookup")
	}
	return r.item, nil
}
func TestManagementLocksThenRejectsTransferredTesteeWithoutWrite(t *testing.T) {
	target := domain.NewTestee(1, "T", domain.GenderUnknown, nil)
	target.SetID(10)
	store := uint64(8)
	target.RestoreStore(&store, 2)
	repo := &lockedManagementRepo{assessmentAttentionRepoStub: assessmentAttentionRepoStub{item: target}}
	svc := NewScopedManagementService(repo, domain.NewEditor(domain.NewValidator(repo)), nil, apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }), &updateScopeStub{})
	ctx := actorctx.WithOperatorOrgID(actorctx.WithGrantingUserID(context.Background(), 9), 1)
	require.Error(t, svc.MarkAsKeyFocus(ctx, 10))
	require.Equal(t, 1, repo.lockCalls)
	require.Zero(t, repo.findByIDCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, target.IsKeyFocus())
	store = 7
	target.RestoreStore(&store, 3)
	require.NoError(t, svc.MarkAsKeyFocus(ctx, 10))
	require.Equal(t, 2, repo.lockCalls)
	require.Equal(t, 1, repo.updateCalls)
}

func TestAtomicProfileUpdatePreservesOmittedFields(t *testing.T) {
	target := domain.NewTestee(1, "Original", domain.GenderUnknown, nil)
	target.SetID(10)
	store := uint64(7)
	target.RestoreStore(&store, 1)
	repo := &lockedManagementRepo{assessmentAttentionRepoStub: assessmentAttentionRepoStub{item: target}}
	transactions := 0
	svc := NewScopedManagementService(repo, domain.NewEditor(domain.NewValidator(repo)), nil, apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { transactions++; return fn(ctx) }), &updateScopeStub{}).(ProfileUpdater)
	ctx := actorctx.WithOperatorOrgID(actorctx.WithGrantingUserID(context.Background(), 9), 1)
	gender, focus := int8(1), true
	require.NoError(t, svc.UpdateProfile(ctx, UpdateProfileDTO{TesteeID: 10, Gender: &gender, IsKeyFocus: &focus}))
	require.Equal(t, "Original", target.Name())
	require.EqualValues(t, 1, target.Gender())
	require.True(t, target.IsKeyFocus())
	require.Equal(t, 1, transactions)
	require.Equal(t, 1, repo.lockCalls)
	require.Equal(t, 1, repo.updateCalls)
	invalid := int8(99)
	require.Error(t, svc.UpdateProfile(ctx, UpdateProfileDTO{TesteeID: 10, Gender: &invalid, IsKeyFocus: &focus}))
	require.Equal(t, 1, repo.updateCalls)
}
