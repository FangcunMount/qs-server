package clinician

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	"testing"

	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

func TestQueryServiceGetBasicByIDDoesNotEnrichCounts(t *testing.T) {
	relationReader := &queryRelationReaderStub{err: errors.New("relation count should not run")}
	assessmentEntryReader := &queryAssessmentEntryReaderStub{err: errors.New("assessment entry count should not run")}
	service := NewQueryService(
		&queryClinicianReaderStub{row: &actorreadmodel.ClinicianRow{ID: 12, OrgID: 91, Name: "Dr. Fang", IsActive: true}},
		relationReader,
		assessmentEntryReader,
	)

	got, err := service.GetBasicByID(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetBasicByID() error = %v", err)
	}
	if got == nil || got.ID != 12 || got.OrgID != 91 {
		t.Fatalf("GetBasicByID() = %+v, want clinician 12 in org 91", got)
	}
	if relationReader.listActiveCalls != 0 {
		t.Fatalf("relation count calls = %d, want 0", relationReader.listActiveCalls)
	}
	if assessmentEntryReader.countCalls != 0 {
		t.Fatalf("assessment entry count calls = %d, want 0", assessmentEntryReader.countCalls)
	}
	if got.AssignedTesteeCount != 0 || got.AssessmentEntryCount != 0 {
		t.Fatalf("counts = (%d, %d), want zero-value counts", got.AssignedTesteeCount, got.AssessmentEntryCount)
	}
}

func TestQueryServiceGetByIDStillEnrichesCounts(t *testing.T) {
	relationReader := &queryRelationReaderStub{ids: []uint64{7, 7, 8}}
	assessmentEntryReader := &queryAssessmentEntryReaderStub{count: 3}
	service := NewQueryService(
		&queryClinicianReaderStub{row: &actorreadmodel.ClinicianRow{ID: 12, OrgID: 91, Name: "Dr. Fang", IsActive: true}},
		relationReader,
		assessmentEntryReader,
	)

	got, err := service.GetByID(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if relationReader.listActiveCalls != 1 {
		t.Fatalf("relation count calls = %d, want 1", relationReader.listActiveCalls)
	}
	if assessmentEntryReader.countCalls != 1 {
		t.Fatalf("assessment entry count calls = %d, want 1", assessmentEntryReader.countCalls)
	}
	if got.AssignedTesteeCount != 2 || got.AssessmentEntryCount != 3 {
		t.Fatalf("counts = (%d, %d), want (2, 3)", got.AssignedTesteeCount, got.AssessmentEntryCount)
	}
}

type queryClinicianReaderStub struct {
	row *actorreadmodel.ClinicianRow
	err error
}

func (s *queryClinicianReaderStub) GetClinician(context.Context, uint64) (*actorreadmodel.ClinicianRow, error) {
	return s.row, s.err
}

func (*queryClinicianReaderStub) FindClinicianByOperator(context.Context, int64, uint64) (*actorreadmodel.ClinicianRow, error) {
	return nil, nil
}

func (*queryClinicianReaderStub) ListClinicians(context.Context, actorreadmodel.ClinicianFilter) ([]actorreadmodel.ClinicianRow, error) {
	return nil, nil
}

func (*queryClinicianReaderStub) CountClinicians(context.Context, actorreadmodel.ClinicianFilter) (int64, error) {
	return 0, nil
}

type queryRelationReaderStub struct {
	ids             []uint64
	err             error
	listActiveCalls int
}

func (*queryRelationReaderStub) ListAssignedTestees(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRow, int64, error) {
	return nil, 0, nil
}

func (s *queryRelationReaderStub) ListActiveTesteeIDsByClinician(context.Context, int64, uint64, []string) ([]uint64, error) {
	s.listActiveCalls++
	return s.ids, s.err
}

func (*queryRelationReaderStub) ListActiveTesteeRelationsByTesteeIDs(context.Context, int64, []uint64, []string) ([]actorreadmodel.TesteeRelationRow, error) {
	return nil, nil
}

func (*queryRelationReaderStub) ListTesteeRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRelationRow, error) {
	return nil, nil
}

func (*queryRelationReaderStub) ListClinicianRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.ClinicianRelationRow, int64, error) {
	return nil, 0, nil
}

func (*queryRelationReaderStub) HasActiveRelationForTestee(context.Context, int64, uint64, uint64, []string) (bool, error) {
	return false, nil
}

type queryAssessmentEntryReaderStub struct {
	count      int64
	err        error
	countCalls int
}

func (*queryAssessmentEntryReaderStub) ListAssessmentEntriesByClinician(context.Context, actorreadmodel.AssessmentEntryFilter) ([]actorreadmodel.AssessmentEntryRow, error) {
	return nil, nil
}

func (s *queryAssessmentEntryReaderStub) CountAssessmentEntriesByClinician(context.Context, int64, uint64) (int64, error) {
	s.countCalls++
	return s.count, s.err
}

func (*queryAssessmentEntryReaderStub) GetAssessmentEntryTitle(context.Context, uint64) (string, error) {
	return "", nil
}

type clinicianQueryScope struct{}

func (clinicianQueryScope) ResolveStoreRange(_ context.Context, org, user int64, resource, action string) (appauthz.StoreRange, error) {
	if org != 1 || user != 9 {
		panic("wrong query company")
	}
	if resource == "qs:actor:collection:clinicians" {
		return appauthz.StoreRange{AllStores: true}, nil
	}
	if resource != "qs:actor:collection:testees" || action != "list" {
		panic("wrong count permission")
	}
	return appauthz.StoreRange{StoreIDs: []uint64{7}}, nil
}

type scopedQueryRelations struct {
	queryRelationReaderStub
	filter actorreadmodel.RelationFilter
}

func (r *scopedQueryRelations) ListAssignedTestees(_ context.Context, f actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRow, int64, error) {
	r.filter = f
	return []actorreadmodel.TesteeRow{{ID: 1}, {ID: 1}, {ID: 2}}, 3, nil
}
func TestHeadquartersClinicianCountUsesSeparateTesteeScope(t *testing.T) {
	reader := &queryClinicianReaderStub{row: &actorreadmodel.ClinicianRow{ID: 12, OrgID: 1}}
	relations := &scopedQueryRelations{}
	service := NewOperatorQueryService(reader, relations, nil, clinicianQueryScope{})
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:*:*:*", "*")
	result, err := service.GetByID(ctx, 12)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssignedTesteeCount != 2 || relations.listActiveCalls != 0 || !relations.filter.RestrictToStoreScope || relations.filter.AllAssignedStores || len(relations.filter.AllowedStoreIDs) != 1 || relations.filter.AllowedStoreIDs[0] != 7 {
		t.Fatalf("unscoped count: %+v %+v", result, relations.filter)
	}
	reader.row.OrgID = 2
	if _, err := service.GetByID(ctx, 12); err == nil {
		t.Fatal("foreign company clinician visible")
	}
	if _, err := service.ListClinicians(context.Background(), ListClinicianDTO{OrgID: 1}); err == nil {
		t.Fatal("missing authority allowed")
	}
}
