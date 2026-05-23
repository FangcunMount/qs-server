package operator

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestActiveOperatorCheckerResolveActiveUsesRequestedOrg(t *testing.T) {
	reader := &activeCheckerReaderStub{
		findRow: &actorreadmodel.OperatorRow{ID: 7, OrgID: 88, UserID: 42, IsActive: true},
	}
	checker := NewActiveOperatorChecker(reader)

	got, err := checker.ResolveActive(context.Background(), 42, 88)
	if err != nil {
		t.Fatalf("ResolveActive() error = %v", err)
	}
	if got == nil || got.ID != 7 || got.OrgID != 88 || got.UserID != 42 {
		t.Fatalf("ResolveActive() = %+v, want operator 7 in org 88", got)
	}
	if reader.findCalls != 1 || reader.listCalls != 0 {
		t.Fatalf("calls find=%d list=%d, want find=1 list=0", reader.findCalls, reader.listCalls)
	}
}

func TestActiveOperatorCheckerResolveActiveUsesSingleMembership(t *testing.T) {
	reader := &activeCheckerReaderStub{
		listRows: []actorreadmodel.OperatorRow{{ID: 7, OrgID: 88, UserID: 42, IsActive: true}},
	}
	checker := NewActiveOperatorChecker(reader)

	got, err := checker.ResolveActive(context.Background(), 42, 0)
	if err != nil {
		t.Fatalf("ResolveActive() error = %v", err)
	}
	if got == nil || got.ID != 7 || got.OrgID != 88 || got.UserID != 42 {
		t.Fatalf("ResolveActive() = %+v, want operator 7 in org 88", got)
	}
	if reader.listCalls != 1 || reader.lastFilter.UserID != 42 || !reader.lastFilter.ActiveOnly || reader.lastFilter.Limit != 2 {
		t.Fatalf("unexpected list filter/calls: calls=%d filter=%+v", reader.listCalls, reader.lastFilter)
	}
}

func TestActiveOperatorCheckerResolveActiveRejectsMultipleMembershipsWithoutRequestedOrg(t *testing.T) {
	reader := &activeCheckerReaderStub{
		listRows: []actorreadmodel.OperatorRow{
			{ID: 7, OrgID: 88, UserID: 42, IsActive: true},
			{ID: 8, OrgID: 99, UserID: 42, IsActive: true},
		},
	}
	checker := NewActiveOperatorChecker(reader)

	_, err := checker.ResolveActive(context.Background(), 42, 0)
	if !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("ResolveActive() error = %v, want ErrInvalidArgument", err)
	}
}

type activeCheckerReaderStub struct {
	findRow    *actorreadmodel.OperatorRow
	findErr    error
	listRows   []actorreadmodel.OperatorRow
	listErr    error
	findCalls  int
	listCalls  int
	lastFilter actorreadmodel.OperatorFilter
}

func (*activeCheckerReaderStub) GetOperator(context.Context, uint64) (*actorreadmodel.OperatorRow, error) {
	return nil, nil
}

func (s *activeCheckerReaderStub) FindOperatorByUser(context.Context, int64, int64) (*actorreadmodel.OperatorRow, error) {
	s.findCalls++
	return s.findRow, s.findErr
}

func (s *activeCheckerReaderStub) ListOperators(_ context.Context, filter actorreadmodel.OperatorFilter) ([]actorreadmodel.OperatorRow, error) {
	s.listCalls++
	s.lastFilter = filter
	return s.listRows, s.listErr
}

func (*activeCheckerReaderStub) CountOperators(context.Context, int64) (int64, error) {
	return 0, nil
}
