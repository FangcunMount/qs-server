package access

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestActionScopeNeverFallsBackToUnscopedAdminSnapshot(t *testing.T) {
	svc := NewTesteeAccessService(&stubOperatorReader{item: actorreadmodel.OperatorRow{OrgID: 1, UserID: 101, IsActive: true}}, nil)
	for _, ctx := range []context.Context{context.Background(), authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{Permissions: []authzapp.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authzapp.AuthorizationModeUnconditional}}})} {
		if _, err := svc.ResolveStoreRange(ctx, 1, 101, "qs:actor:collection:testees", "read"); !cberrors.IsCode(err, code.ErrPermissionDenied) {
			t.Fatalf("legacy or missing scope accepted: %v", err)
		}
	}
}

type stubOperatorReader struct {
	item actorreadmodel.OperatorRow
}

func (s *stubOperatorReader) GetOperator(context.Context, uint64) (*actorreadmodel.OperatorRow, error) {
	panic("unexpected call")
}
func (s *stubOperatorReader) FindOperatorByUser(context.Context, int64, int64) (*actorreadmodel.OperatorRow, error) {
	return &s.item, nil
}
func (s *stubOperatorReader) ListOperators(context.Context, actorreadmodel.OperatorFilter) ([]actorreadmodel.OperatorRow, error) {
	panic("unexpected call")
}
func (s *stubOperatorReader) CountOperators(context.Context, int64) (int64, error) {
	panic("unexpected call")
}

type stubTesteeReader struct {
	item actorreadmodel.TesteeRow
}

func (s *stubTesteeReader) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	return &s.item, nil
}
func (s *stubTesteeReader) FindTesteeByProfile(context.Context, int64, uint64) (*actorreadmodel.TesteeRow, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) ListTestees(context.Context, actorreadmodel.TesteeFilter) ([]actorreadmodel.TesteeRow, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) CountTestees(context.Context, actorreadmodel.TesteeFilter) (int64, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) ListTesteesByProfileIDs(context.Context, []uint64, int, int) ([]actorreadmodel.TesteeRow, error) {
	panic("unexpected call")
}
func (s *stubTesteeReader) CountTesteesByProfileIDs(context.Context, []uint64) (int64, error) {
	panic("unexpected call")
}
