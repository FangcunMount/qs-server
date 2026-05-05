package workbench

import (
	"context"
	"testing"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestServiceListHighRiskQueueUsesLatestRiskRowsAndExcludesNonHighRisk(t *testing.T) {
	base := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	testees := &testeeReaderStub{rowsByID: map[uint64]actorreadmodel.TesteeRow{
		1: testeeRow(1, "A"),
		2: testeeRow(2, "B"),
		3: testeeRow(3, "C"),
	}}
	latestRisks := &latestRiskReaderStub{rows: []evaluationreadmodel.LatestRiskRow{
		{AssessmentID: 102, OrgID: 9, TesteeID: 2, RiskLevel: "severe", OccurredAt: base.Add(2 * time.Hour)},
		{AssessmentID: 101, OrgID: 9, TesteeID: 1, RiskLevel: "high", OccurredAt: base.Add(time.Hour)},
	}}
	svc := newTestService(testees, latestRisks, &followUpReaderStub{}, &assignmentHydratorStub{})

	page, err := svc.ListQueue(context.Background(), ListQueueDTO{
		Scope:     Scope{Kind: ScopeKindClinicianMe, OrgID: 9, OperatorUserID: 701},
		QueueType: QueueTypeHighRisk,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListQueue returned error: %v", err)
	}

	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("total/items = %d/%d, want 2/2", page.Total, len(page.Items))
	}
	if page.Items[0].Testee.ID != 2 || page.Items[0].RiskLevel != "severe" || page.Items[0].ReasonCode != "latest_risk_severe" {
		t.Fatalf("unexpected first item: %#v", page.Items[0])
	}
	if page.Items[1].Testee.ID != 1 || page.Items[1].RiskLevel != "high" {
		t.Fatalf("unexpected second item: %#v", page.Items[1])
	}
	if !latestRisks.lastFilter.RestrictToTesteeIDs || len(latestRisks.lastFilter.TesteeIDs) != 3 {
		t.Fatalf("latest risk filter did not keep assigned scope: %#v", latestRisks.lastFilter)
	}
}

func TestServiceListHighRiskQueueIgnoresManualRiskTags(t *testing.T) {
	testees := &testeeReaderStub{rowsByID: map[uint64]actorreadmodel.TesteeRow{
		1: func() actorreadmodel.TesteeRow {
			row := testeeRow(1, "A")
			row.Tags = []string{"risk_high"}
			return row
		}(),
	}}
	latestRisks := &latestRiskReaderStub{}
	svc := newTestService(testees, latestRisks, &followUpReaderStub{}, &assignmentHydratorStub{})

	page, err := svc.ListQueue(context.Background(), ListQueueDTO{
		Scope:     Scope{Kind: ScopeKindClinicianMe, OrgID: 9, OperatorUserID: 701},
		QueueType: QueueTypeHighRisk,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListQueue returned error: %v", err)
	}

	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("high risk queue should ignore manual risk tags, got %#v", page)
	}
	if !latestRisks.lastFilter.RestrictToTesteeIDs {
		t.Fatalf("latest risk filter should still be the queue fact source: %#v", latestRisks.lastFilter)
	}
}

func TestServiceListFollowUpQueueReturnsUrgentTaskPerTestee(t *testing.T) {
	plannedAt := time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)
	expireAt := plannedAt.Add(24 * time.Hour)
	testees := &testeeReaderStub{rowsByID: map[uint64]actorreadmodel.TesteeRow{
		1: testeeRow(1, "A"),
		2: testeeRow(2, "B"),
	}}
	followUps := &followUpReaderStub{page: planreadmodel.TaskPage{
		Items: []planreadmodel.TaskRow{
			{ID: 202, PlanID: 302, OrgID: 9, TesteeID: 2, ScaleCode: "SDS", PlannedAt: plannedAt, ExpireAt: &expireAt, Status: "expired", EntryURL: "https://entry/2"},
			{ID: 201, PlanID: 301, OrgID: 9, TesteeID: 1, ScaleCode: "SAS", PlannedAt: plannedAt, Status: "opened", EntryURL: "https://entry/1"},
		},
		Total: 2,
	}}
	svc := newTestService(testees, &latestRiskReaderStub{}, followUps, &assignmentHydratorStub{})

	page, err := svc.ListQueue(context.Background(), ListQueueDTO{
		Scope:     Scope{Kind: ScopeKindClinicianMe, OrgID: 9, OperatorUserID: 701},
		QueueType: QueueTypeFollowUp,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListQueue returned error: %v", err)
	}

	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("total/items = %d/%d, want 2/2", page.Total, len(page.Items))
	}
	if page.Items[0].Testee.ID != 2 || page.Items[0].ReasonCode != "follow_up_expired" {
		t.Fatalf("unexpected expired item: %#v", page.Items[0])
	}
	if page.Items[0].Task == nil || page.Items[0].Task.TaskID != 202 || page.Items[0].Task.EntryURL != "https://entry/2" {
		t.Fatalf("unexpected task summary: %#v", page.Items[0].Task)
	}
	if page.Items[1].Testee.ID != 1 || page.Items[1].ReasonCode != "follow_up_opened" {
		t.Fatalf("unexpected opened item: %#v", page.Items[1])
	}
	if !followUps.lastFilter.RestrictToTesteeIDs || len(followUps.lastFilter.TesteeIDs) != 3 {
		t.Fatalf("follow-up filter did not keep assigned scope: %#v", followUps.lastFilter)
	}
}

func TestServiceListKeyFocusQueueUsesAssignedScope(t *testing.T) {
	testees := &testeeReaderStub{
		listRows: []actorreadmodel.TesteeRow{{ID: 2, OrgID: 9, Name: "B", IsKeyFocus: true}},
		count:    1,
	}
	svc := newTestService(testees, &latestRiskReaderStub{}, &followUpReaderStub{}, &assignmentHydratorStub{})

	page, err := svc.ListQueue(context.Background(), ListQueueDTO{
		Scope:     Scope{Kind: ScopeKindClinicianMe, OrgID: 9, OperatorUserID: 701},
		QueueType: QueueTypeKeyFocus,
		Page:      2,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("ListQueue returned error: %v", err)
	}

	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Testee.ID != 2 {
		t.Fatalf("unexpected key focus page: %#v", page)
	}
	if !testees.lastFilter.RestrictToAccessScope || len(testees.lastFilter.AccessibleTesteeIDs) != 3 {
		t.Fatalf("filter did not keep assigned scope: %#v", testees.lastFilter)
	}
	if testees.lastFilter.Offset != 20 || testees.lastFilter.Limit != 20 {
		t.Fatalf("offset/limit = %d/%d, want 20/20", testees.lastFilter.Offset, testees.lastFilter.Limit)
	}
}

func TestServiceGetSummaryReturnsEmptyWhenOperatorIsNotBoundToClinician(t *testing.T) {
	svc := NewService(
		&operatorQueryStub{err: cberrors.WithCode(code.ErrUserNotFound, "operator not found")},
		&clinicianQueryStub{},
		&assignmentReaderStub{},
		&assignmentHydratorStub{},
		&testeeReaderStub{},
		&latestRiskReaderStub{},
		&followUpReaderStub{},
	)

	summary, err := svc.GetSummary(context.Background(), Scope{Kind: ScopeKindClinicianMe, OrgID: 9, OperatorUserID: 701})
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
	if summary.Counts != (QueueCounts{}) {
		t.Fatalf("summary = %#v, want zero", summary)
	}
}

func TestServiceListQueueRejectsUnknownQueueType(t *testing.T) {
	svc := newTestService(&testeeReaderStub{}, &latestRiskReaderStub{}, &followUpReaderStub{}, &assignmentHydratorStub{})

	_, err := svc.ListQueue(context.Background(), ListQueueDTO{
		Scope:     Scope{Kind: ScopeKindClinicianMe, OrgID: 9, OperatorUserID: 701},
		QueueType: QueueType("unknown"),
	})
	if err == nil || !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestServiceListOrgAdminHighRiskQueueDoesNotRequireClinicianBindingAndHydratesAssignments(t *testing.T) {
	base := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	testees := &testeeReaderStub{rowsByID: map[uint64]actorreadmodel.TesteeRow{
		4: testeeRow(4, "D"),
	}}
	latestRisks := &latestRiskReaderStub{rows: []evaluationreadmodel.LatestRiskRow{
		{AssessmentID: 204, OrgID: 9, TesteeID: 4, RiskLevel: "high", OccurredAt: base},
	}}
	assignments := &assignmentHydratorStub{rows: []actorreadmodel.TesteeRelationRow{
		{
			Relation: actorreadmodel.RelationRow{OrgID: 9, TesteeID: 4, ClinicianID: 44, RelationType: "primary", BoundAt: base},
			Clinician: actorreadmodel.ClinicianRow{
				ID:            44,
				OrgID:         9,
				Name:          "Dr. Admin",
				Department:    "心理科",
				ClinicianType: "doctor",
				IsActive:      true,
			},
		},
	}}
	svc := newTestService(testees, latestRisks, &followUpReaderStub{}, assignments)

	page, err := svc.ListQueue(context.Background(), ListQueueDTO{
		Scope:     Scope{Kind: ScopeKindOrgAdmin, OrgID: 9},
		QueueType: QueueTypeHighRisk,
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("ListQueue returned error: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Testee.ID != 4 {
		t.Fatalf("unexpected admin high risk page: %#v", page)
	}
	if latestRisks.lastFilter.RestrictToTesteeIDs || len(latestRisks.lastFilter.TesteeIDs) != 0 {
		t.Fatalf("admin latest risk filter should be whole org: %#v", latestRisks.lastFilter)
	}
	if page.Items[0].PrimaryClinician == nil || page.Items[0].PrimaryClinician.ID != 44 {
		t.Fatalf("primary clinician not hydrated: %#v", page.Items[0])
	}
	if page.Items[0].IsUnassigned == nil || *page.Items[0].IsUnassigned {
		t.Fatalf("is_unassigned = %#v, want false", page.Items[0].IsUnassigned)
	}
}

func TestServiceListOrgAdminQueueWithClinicianFilterUsesAssignedScope(t *testing.T) {
	testees := &testeeReaderStub{
		listRows: []actorreadmodel.TesteeRow{{ID: 2, OrgID: 9, Name: "B", IsKeyFocus: true}},
		count:    1,
	}
	svc := NewService(
		&operatorQueryStub{},
		&clinicianQueryStub{},
		&assignmentReaderStub{ids: []uint64{2}},
		&assignmentHydratorStub{},
		testees,
		&latestRiskReaderStub{},
		&followUpReaderStub{},
	)
	clinicianID := uint64(20)

	page, err := svc.ListQueue(context.Background(), ListQueueDTO{
		Scope:     Scope{Kind: ScopeKindOrgAdmin, OrgID: 9, ClinicianID: &clinicianID},
		QueueType: QueueTypeKeyFocus,
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("ListQueue returned error: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Testee.ID != 2 {
		t.Fatalf("unexpected clinician-filtered admin page: %#v", page)
	}
	if !testees.lastFilter.RestrictToAccessScope || len(testees.lastFilter.AccessibleTesteeIDs) != 1 || testees.lastFilter.AccessibleTesteeIDs[0] != 2 {
		t.Fatalf("key focus filter did not restrict to clinician assignments: %#v", testees.lastFilter)
	}
}

func newTestService(
	testees *testeeReaderStub,
	latestRisks *latestRiskReaderStub,
	followUps *followUpReaderStub,
	assignments *assignmentHydratorStub,
) Service {
	return NewService(
		&operatorQueryStub{result: &operatorApp.OperatorResult{ID: 10, OrgID: 9, UserID: 701, IsActive: true}},
		&clinicianQueryStub{result: &clinicianApp.ClinicianResult{ID: 20, OrgID: 9, IsActive: true}},
		&assignmentReaderStub{ids: []uint64{1, 2, 3}},
		assignments,
		testees,
		latestRisks,
		followUps,
	)
}

type operatorQueryStub struct {
	result *operatorApp.OperatorResult
	err    error
}

func (s *operatorQueryStub) GetByUser(context.Context, int64, int64) (*operatorApp.OperatorResult, error) {
	return s.result, s.err
}

type clinicianQueryStub struct {
	result *clinicianApp.ClinicianResult
	err    error
}

func (s *clinicianQueryStub) GetByOperator(context.Context, int64, uint64) (*clinicianApp.ClinicianResult, error) {
	return s.result, s.err
}

type assignmentReaderStub struct {
	ids []uint64
	err error
}

func (s *assignmentReaderStub) ListAssignedTesteeIDs(context.Context, int64, uint64) ([]uint64, error) {
	return append([]uint64(nil), s.ids...), s.err
}

type testeeReaderStub struct {
	rowsByID   map[uint64]actorreadmodel.TesteeRow
	listRows   []actorreadmodel.TesteeRow
	count      int64
	lastFilter actorreadmodel.TesteeFilter
}

func (s *testeeReaderStub) ListTesteesByIDs(_ context.Context, _ int64, ids []uint64) ([]actorreadmodel.TesteeRow, error) {
	rows := make([]actorreadmodel.TesteeRow, 0, len(ids))
	for _, id := range ids {
		if row, ok := s.rowsByID[id]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (s *testeeReaderStub) ListTestees(_ context.Context, filter actorreadmodel.TesteeFilter) ([]actorreadmodel.TesteeRow, error) {
	s.lastFilter = filter
	return append([]actorreadmodel.TesteeRow(nil), s.listRows...), nil
}

func (s *testeeReaderStub) CountTestees(_ context.Context, filter actorreadmodel.TesteeFilter) (int64, error) {
	s.lastFilter = filter
	return s.count, nil
}

type latestRiskReaderStub struct {
	rows       []evaluationreadmodel.LatestRiskRow
	err        error
	lastFilter evaluationreadmodel.LatestRiskQueueFilter
}

func (s *latestRiskReaderStub) ListLatestRisksByTesteeIDs(context.Context, evaluationreadmodel.LatestRiskFilter) ([]evaluationreadmodel.LatestRiskRow, error) {
	return append([]evaluationreadmodel.LatestRiskRow(nil), s.rows...), s.err
}

func (s *latestRiskReaderStub) ListLatestRiskQueue(_ context.Context, filter evaluationreadmodel.LatestRiskQueueFilter, page evaluationreadmodel.PageRequest) (evaluationreadmodel.LatestRiskPage, error) {
	s.lastFilter = filter
	if s.err != nil {
		return evaluationreadmodel.LatestRiskPage{}, s.err
	}
	return evaluationreadmodel.LatestRiskPage{
		Items:    append([]evaluationreadmodel.LatestRiskRow(nil), s.rows...),
		Total:    int64(len(s.rows)),
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

type followUpReaderStub struct {
	page       planreadmodel.TaskPage
	err        error
	lastFilter planreadmodel.FollowUpQueueFilter
}

func (s *followUpReaderStub) ListFollowUpQueueTasks(_ context.Context, filter planreadmodel.FollowUpQueueFilter, _ planreadmodel.PageRequest) (planreadmodel.TaskPage, error) {
	s.lastFilter = filter
	return s.page, s.err
}

type assignmentHydratorStub struct {
	rows []actorreadmodel.TesteeRelationRow
	err  error
}

func (s *assignmentHydratorStub) ListActiveTesteeRelationsByTesteeIDs(context.Context, int64, []uint64, []string) ([]actorreadmodel.TesteeRelationRow, error) {
	return append([]actorreadmodel.TesteeRelationRow(nil), s.rows...), s.err
}

func testeeRow(id uint64, name string) actorreadmodel.TesteeRow {
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	return actorreadmodel.TesteeRow{
		ID:        id,
		OrgID:     9,
		Name:      name,
		Gender:    1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
