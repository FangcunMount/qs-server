package clinician

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	"testing"
	"time"

	cbErrors "github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestAssignTesteeNormalizesAssignedToAttending(t *testing.T) {
	relationRepo := &relationshipServiceRelationRepo{}
	svc := &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(10)},
		testeeRepo:    &relationshipServiceTesteeRepo{item: makeTestee(20)},
		uow:           passthroughTxRunner{},
	}

	result, err := svc.AssignTestee(context.Background(), AssignTesteeDTO{
		OrgID:        1,
		ClinicianID:  10,
		TesteeID:     20,
		RelationType: string(domainRelation.RelationTypeAssigned),
	})
	if err != nil {
		t.Fatalf("expected assign testee to succeed: %v", err)
	}
	if result.RelationType != string(domainRelation.RelationTypeAttending) {
		t.Fatalf("expected assigned to normalize to attending, got %s", result.RelationType)
	}
	if relationRepo.saved == nil || relationRepo.saved.RelationType() != domainRelation.RelationTypeAttending {
		t.Fatalf("expected saved relation type to be attending")
	}
}

func TestTransferPrimaryUnbindsExistingPrimary(t *testing.T) {
	existingPrimary := domainRelation.NewClinicianTesteeRelation(
		1,
		domainClinician.ID(10),
		domainTestee.ID(20),
		domainRelation.RelationTypePrimary,
		domainRelation.SourceTypeManual,
		nil,
		true,
		time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		nil,
	)
	existingPrimary.SetID(domainRelation.ID(9001))

	relationRepo := &relationshipServiceRelationRepo{
		activePrimaryByTestee: existingPrimary,
	}
	svc := &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(11)},
		testeeRepo:    &relationshipServiceTesteeRepo{item: makeTestee(20)},
		uow:           passthroughTxRunner{},
	}

	result, err := svc.TransferPrimary(context.Background(), TransferPrimaryDTO{
		OrgID:         1,
		ToClinicianID: 11,
		TesteeID:      20,
	})
	if err != nil {
		t.Fatalf("expected transfer primary to succeed: %v", err)
	}
	if relationRepo.updated == nil || relationRepo.updated.IsActive() {
		t.Fatalf("expected existing primary relation to be unbound")
	}
	if relationRepo.saved == nil || relationRepo.saved.RelationType() != domainRelation.RelationTypePrimary {
		t.Fatalf("expected new primary relation to be saved")
	}
	if relationRepo.saved.SourceType() != domainRelation.SourceTypeTransfer {
		t.Fatalf("expected default transfer source type, got %s", relationRepo.saved.SourceType())
	}
	if result.ClinicianID != 11 {
		t.Fatalf("expected transferred primary to target clinician 11, got %d", result.ClinicianID)
	}
}

func TestAssignPrimaryReplacesExistingAccessRelation(t *testing.T) {
	existingAccess := domainRelation.NewClinicianTesteeRelation(
		1,
		domainClinician.ID(11),
		domainTestee.ID(20),
		domainRelation.RelationTypeAttending,
		domainRelation.SourceTypeManual,
		nil,
		true,
		time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		nil,
	)
	existingAccess.SetID(domainRelation.ID(9002))

	relationRepo := &relationshipServiceRelationRepo{
		activeAccessByTypes: existingAccess,
	}
	svc := &relationshipService{
		relationRepo:  relationRepo,
		clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(11)},
		testeeRepo:    &relationshipServiceTesteeRepo{item: makeTestee(20)},
		uow:           passthroughTxRunner{},
	}

	result, err := svc.AssignPrimary(context.Background(), AssignTesteeDTO{
		OrgID:       1,
		ClinicianID: 11,
		TesteeID:    20,
	})
	if err != nil {
		t.Fatalf("expected assign primary to succeed: %v", err)
	}
	if len(relationRepo.updatedItems) != 1 || relationRepo.updatedItems[0].IsActive() {
		t.Fatalf("expected existing access relation to be unbound")
	}
	if relationRepo.saved == nil || relationRepo.saved.RelationType() != domainRelation.RelationTypePrimary {
		t.Fatalf("expected new primary relation to be saved")
	}
	if result.RelationType != string(domainRelation.RelationTypePrimary) {
		t.Fatalf("expected primary result, got %s", result.RelationType)
	}
}

type passthroughTxRunner struct{}

func (passthroughTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestListAssignedTesteesUsesReadModel(t *testing.T) {
	relationReader := &relationshipServiceRelationReader{
		assignedRows: []actorreadmodel.TesteeRow{
			{ID: 21, OrgID: 1, Name: "testee-21"},
			{ID: 20, OrgID: 1, Name: "testee-20"},
		},
		assignedTotal: 2,
	}
	svc := &relationshipService{
		relationReader: relationReader,
	}

	result, err := svc.ListAssignedTestees(authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:evaluation:collection:assessments", "read"), ListAssignedTesteeDTO{
		OrgID:       1,
		ClinicianID: 10,
		Offset:      0,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("expected list assigned testees to succeed: %v", err)
	}
	if relationReader.listAssignedCalls != 1 {
		t.Fatalf("expected read model to be called once, got %d", relationReader.listAssignedCalls)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 assigned testees, got %d", len(result.Items))
	}
	if result.Items[0].ID != 21 || result.Items[1].ID != 20 {
		t.Fatalf("expected relation order to be preserved, got %+v", result.Items)
	}
}

type relationshipAssessmentSummaryReader struct {
	requested []uint64
	calls     int
	err       error
	values    map[uint64]actorreadmodel.AssessmentSummary
}

func (s *relationshipAssessmentSummaryReader) ReadAssessmentSummaries(_ context.Context, _ int64, ids []uint64) (map[uint64]actorreadmodel.AssessmentSummary, error) {
	s.calls++
	s.requested = append([]uint64(nil), ids...)
	return s.values, s.err
}

func TestListAssignedTesteesUsesOneEvaluationSummaryBatch(t *testing.T) {
	stale := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	relationReader := &relationshipServiceRelationReader{
		assignedRows:  []actorreadmodel.TesteeRow{{ID: 21, OrgID: 1, StoreID: storePtr(7), Name: "testee", TotalAssessments: 99, LastAssessmentAt: &stale, LastRiskLevel: "low"}},
		assignedTotal: 1,
	}
	summary := &relationshipAssessmentSummaryReader{values: map[uint64]actorreadmodel.AssessmentSummary{
		21: {TesteeID: 21, TotalEvaluated: 3, LastEvaluatedAt: &latest, RiskLevel: "high"},
	}}
	svc := &relationshipService{relationReader: relationReader, summaryReader: summary, summaryScope: relationshipSummaryScope{}}

	result, err := svc.ListAssignedTestees(authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:evaluation:collection:assessments", "read"), ListAssignedTesteeDTO{OrgID: 1, ClinicianID: 10, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if summary.calls != 1 {
		t.Fatalf("summary calls=%d want=1", summary.calls)
	}
	item := result.Items[0]
	if item.TotalAssessments != 3 || item.LastAssessmentAt == nil || !item.LastAssessmentAt.Equal(latest) || item.LastRiskLevel != "high" {
		t.Fatalf("assigned testee summary=%+v", item)
	}

	summary.err = errors.New("summary database unavailable")
	if _, err := svc.ListAssignedTestees(authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:evaluation:collection:assessments", "read"), ListAssignedTesteeDTO{OrgID: 1, ClinicianID: 10, Limit: 10}); err == nil {
		t.Fatal("summary query failure must fail the page")
	}
}

func TestListClinicianRelationsUsesReadModel(t *testing.T) {
	relationReader := &relationshipServiceRelationReader{
		clinicianRelations: []actorreadmodel.ClinicianRelationRow{
			{
				Relation: actorreadmodel.RelationRow{ID: 30, OrgID: 1, ClinicianID: 10, TesteeID: 30, RelationType: string(domainRelation.RelationTypeAttending), IsActive: true},
				Testee:   actorreadmodel.TesteeRow{ID: 30, OrgID: 1, Name: "testee-30"},
			},
			{
				Relation: actorreadmodel.RelationRow{ID: 31, OrgID: 1, ClinicianID: 10, TesteeID: 31, RelationType: string(domainRelation.RelationTypeAttending), IsActive: true},
				Testee:   actorreadmodel.TesteeRow{ID: 31, OrgID: 1, Name: "testee-31"},
			},
		},
		clinicianRelationsTotal: 2,
	}
	svc := &relationshipService{
		relationReader: relationReader,
	}

	result, err := svc.ListClinicianRelations(context.Background(), ListClinicianRelationDTO{
		OrgID:       1,
		ClinicianID: 10,
		ActiveOnly:  true,
		Offset:      0,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("expected list clinician relations to succeed: %v", err)
	}
	if relationReader.listClinicianRelationsCalls != 1 {
		t.Fatalf("expected read model to be called once, got %d", relationReader.listClinicianRelationsCalls)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 clinician relations, got %d", len(result.Items))
	}
	if result.Items[0].Testee.ID != 30 || result.Items[1].Testee.ID != 31 {
		t.Fatalf("expected relation order to be preserved, got %+v", result.Items)
	}
}

func makeActiveClinician(id uint64) *domainClinician.Clinician {
	item := domainClinician.NewClinician(1, "clinician", "", "", domainClinician.TypeCounselor, "", true)
	item.SetID(domainClinician.ID(id))
	return item
}

func makeTestee(id uint64) *domainTestee.Testee {
	item := domainTestee.NewTestee(1, "testee", domainTestee.GenderMale, nil)
	item.SetID(domainTestee.ID(id))
	return item
}

type relationshipServiceRelationRepo struct {
	saved                 *domainRelation.ClinicianTesteeRelation
	updated               *domainRelation.ClinicianTesteeRelation
	updatedItems          []*domainRelation.ClinicianTesteeRelation
	activePrimaryByTestee *domainRelation.ClinicianTesteeRelation
	activeAccessByTypes   *domainRelation.ClinicianTesteeRelation
	activeByClinician     []*domainRelation.ClinicianTesteeRelation
	historyByClinician    []*domainRelation.ClinicianTesteeRelation
}

func (s *relationshipServiceRelationRepo) Save(_ context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	s.saved = item
	return nil
}

func (s *relationshipServiceRelationRepo) Update(_ context.Context, item *domainRelation.ClinicianTesteeRelation) error {
	s.updated = item
	s.updatedItems = append(s.updatedItems, item)
	return nil
}

func (s *relationshipServiceRelationRepo) FindByID(context.Context, domainRelation.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) FindActive(context.Context, int64, domainClinician.ID, domainTestee.ID, domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) FindActivePrimaryByTestee(context.Context, int64, domainTestee.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	if s.activePrimaryByTestee == nil {
		return nil, cbErrors.WithCode(code.ErrUserNotFound, "primary relation not found")
	}
	return s.activePrimaryByTestee, nil
}

func (s *relationshipServiceRelationRepo) FindActiveByTypes(context.Context, int64, domainClinician.ID, domainTestee.ID, []domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	if s.activeAccessByTypes != nil {
		return s.activeAccessByTypes, nil
	}
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "relation not found")
}

func (s *relationshipServiceRelationRepo) ListActiveByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType, int, int) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return s.activeByClinician, nil
}

func (s *relationshipServiceRelationRepo) ListHistoryByClinician(context.Context, int64, domainClinician.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return s.historyByClinician, nil
}

func (s *relationshipServiceRelationRepo) CountActiveByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType) (int64, error) {
	return int64(len(s.activeByClinician)), nil
}

func (s *relationshipServiceRelationRepo) ListActiveByTestee(context.Context, int64, domainTestee.ID, []domainRelation.RelationType) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) ListHistoryByTestee(context.Context, int64, domainTestee.ID) ([]*domainRelation.ClinicianTesteeRelation, error) {
	return nil, nil
}

func (s *relationshipServiceRelationRepo) HasActiveRelationForTestee(context.Context, int64, domainClinician.ID, domainTestee.ID, []domainRelation.RelationType) (bool, error) {
	return false, nil
}

func (s *relationshipServiceRelationRepo) ListActiveTesteeIDsByClinician(context.Context, int64, domainClinician.ID, []domainRelation.RelationType) ([]domainTestee.ID, error) {
	return nil, nil
}

type relationshipServiceRelationReader struct {
	assignedRows                 []actorreadmodel.TesteeRow
	assignedTotal                int64
	clinicianRelations           []actorreadmodel.ClinicianRelationRow
	clinicianRelationsTotal      int64
	testeeRelations              []actorreadmodel.TesteeRelationRow
	activeTesteeIDs              []uint64
	listAssignedCalls            int
	listClinicianRelationsCalls  int
	listTesteeRelationsCalls     int
	listActiveTesteeIDsCalls     int
	hasActiveRelationForTesteeFn func(context.Context, int64, uint64, uint64, []string) (bool, error)
}

func (s *relationshipServiceRelationReader) ListAssignedTestees(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRow, int64, error) {
	s.listAssignedCalls++
	return s.assignedRows, s.assignedTotal, nil
}

func (s *relationshipServiceRelationReader) ListActiveTesteeIDsByClinician(context.Context, int64, uint64, []string) ([]uint64, error) {
	s.listActiveTesteeIDsCalls++
	return s.activeTesteeIDs, nil
}

func (s *relationshipServiceRelationReader) ListActiveTesteeRelationsByTesteeIDs(context.Context, int64, []uint64, []string) ([]actorreadmodel.TesteeRelationRow, error) {
	return nil, nil
}

func (s *relationshipServiceRelationReader) ListTesteeRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.TesteeRelationRow, error) {
	s.listTesteeRelationsCalls++
	return s.testeeRelations, nil
}

func (s *relationshipServiceRelationReader) ListClinicianRelations(context.Context, actorreadmodel.RelationFilter) ([]actorreadmodel.ClinicianRelationRow, int64, error) {
	s.listClinicianRelationsCalls++
	return s.clinicianRelations, s.clinicianRelationsTotal, nil
}

func (s *relationshipServiceRelationReader) HasActiveRelationForTestee(ctx context.Context, orgID int64, clinicianID, testeeID uint64, relationTypes []string) (bool, error) {
	if s.hasActiveRelationForTesteeFn != nil {
		return s.hasActiveRelationForTesteeFn(ctx, orgID, clinicianID, testeeID, relationTypes)
	}
	return false, nil
}

type relationshipServiceClinicianRepo struct {
	item *domainClinician.Clinician
}

func (s *relationshipServiceClinicianRepo) Save(context.Context, *domainClinician.Clinician) error {
	return nil
}

func (s *relationshipServiceClinicianRepo) Update(context.Context, *domainClinician.Clinician) error {
	return nil
}

func (s *relationshipServiceClinicianRepo) FindByID(context.Context, domainClinician.ID) (*domainClinician.Clinician, error) {
	return s.item, nil
}

func (s *relationshipServiceClinicianRepo) FindByOperator(context.Context, int64, uint64) (*domainClinician.Clinician, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "clinician not found")
}

func (s *relationshipServiceClinicianRepo) ListByOrg(context.Context, int64, int, int) ([]*domainClinician.Clinician, error) {
	return nil, nil
}

func (s *relationshipServiceClinicianRepo) Count(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *relationshipServiceClinicianRepo) Delete(context.Context, domainClinician.ID) error {
	return nil
}

type relationshipServiceTesteeRepo struct {
	item          *domainTestee.Testee
	findByIDCalls int
}

func (s *relationshipServiceTesteeRepo) Save(context.Context, *domainTestee.Testee) error {
	return nil
}

func (s *relationshipServiceTesteeRepo) Update(context.Context, *domainTestee.Testee) error {
	return nil
}

func (s *relationshipServiceTesteeRepo) FindByID(_ context.Context, id domainTestee.ID) (*domainTestee.Testee, error) {
	s.findByIDCalls++
	return s.item, nil
}

func (s *relationshipServiceTesteeRepo) FindByProfile(context.Context, int64, uint64) (*domainTestee.Testee, error) {
	return nil, cbErrors.WithCode(code.ErrUserNotFound, "testee not found")
}

func (s *relationshipServiceTesteeRepo) Delete(context.Context, domainTestee.ID) error {
	return nil
}

func storePtr(id uint64) *uint64 { return &id }

type relationshipSummaryScope struct{}

func (relationshipSummaryScope) ResolveStoreRange(_ context.Context, org, user int64, resource, action string) (appauthz.StoreRange, error) {
	if org != 1 || user != 9 || resource != appauthz.AssessmentResource || action != "read" {
		panic("wrong summary authority")
	}
	return appauthz.StoreRange{StoreIDs: []uint64{7}}, nil
}

func TestRelationshipSummaryUsesItsOwnScopeAndClearsUnpermittedFields(t *testing.T) {
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), appauthz.AssessmentResource, "read")
	summary := &relationshipAssessmentSummaryReader{values: map[uint64]actorreadmodel.AssessmentSummary{21: {TotalEvaluated: 3, RiskLevel: "high"}, 22: {TotalEvaluated: 99, RiskLevel: "high"}, 23: {TotalEvaluated: 99, RiskLevel: "high"}}}
	svc := &relationshipService{summaryReader: summary, summaryScope: relationshipSummaryScope{}}
	rows := []actorreadmodel.TesteeRow{{ID: 21, OrgID: 1, StoreID: storePtr(7), TotalAssessments: 88}, {ID: 22, OrgID: 1, StoreID: storePtr(8), TotalAssessments: 88, LastRiskLevel: "stale"}, {ID: 23, OrgID: 1, TotalAssessments: 88, LastRiskLevel: "stale"}}
	if err := svc.enrichAssignedRows(ctx, 1, rows); err != nil {
		t.Fatal(err)
	}
	if len(summary.requested) != 1 || summary.requested[0] != 21 || rows[0].TotalAssessments != 3 {
		t.Fatalf("wrong authorized summary set %v", summary.requested)
	}
	for _, row := range rows[1:] {
		if row.TotalAssessments != 0 || row.LastRiskLevel != "" || row.LastAssessmentAt != nil {
			t.Fatal("outside-store or unassigned summary leaked")
		}
	}
	summary.calls = 0
	svc.summaryScope = nil
	rows[0].LastRiskLevel = "stale"
	if err := svc.enrichAssignedRows(ctx, 1, rows); err != nil || summary.calls != 0 || rows[0].LastRiskLevel != "" || rows[0].TotalAssessments != 0 {
		t.Fatal("missing scope reused cached result")
	}
}

type relationReadScope struct{ action string }

func (r relationReadScope) ResolveStoreRange(_ context.Context, org, user int64, resource, action string) (appauthz.StoreRange, error) {
	if org != 1 || user != 9 || resource != "qs:actor:collection:testees" || action != r.action {
		panic("wrong relationship read authority")
	}
	return appauthz.StoreRange{StoreIDs: []uint64{7}}, nil
}
func TestOperatorRelationFilterRequiresExplicitCompanyAndAction(t *testing.T) {
	const resource = "qs:actor:collection:testees"
	for _, action := range []string{"read", "list"} {
		svc := &relationshipService{operatorOnly: true, operatorScope: relationReadScope{action: action}}
		ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), resource, action)
		filter, err := svc.operatorRelationFilter(ctx, actorreadmodel.RelationFilter{OrgID: 1, Offset: 4, Limit: 2}, action)
		if err != nil || !filter.RestrictToStoreScope || len(filter.AllowedStoreIDs) != 1 || filter.AllowedStoreIDs[0] != 7 || filter.Offset != 4 || filter.Limit != 2 {
			t.Fatalf("filter=%+v err=%v", filter, err)
		}
		for _, denied := range []context.Context{context.Background(), actorctx.WithOperatorOrgID(ctx, 2), actorctx.WithGrantingUserID(ctx, 0)} {
			if _, err := svc.operatorRelationFilter(denied, actorreadmodel.RelationFilter{OrgID: 1}, action); err == nil {
				t.Fatal("missing authority allowed")
			}
		}
		svc.operatorScope = nil
		if _, err := svc.operatorRelationFilter(ctx, actorreadmodel.RelationFilter{OrgID: 1}, action); err == nil {
			t.Fatal("missing resolver allowed")
		}
	}
}

type lockedRelationTesteeRepo struct {
	relationshipServiceTesteeRepo
	locks int
}

func (r *lockedRelationTesteeRepo) FindByIDForUpdate(_ context.Context, org int64, id domainTestee.ID) (*domainTestee.Testee, error) {
	r.locks++
	if org != 1 || id != 20 {
		panic("wrong ownership lock")
	}
	return r.item, nil
}
func TestOperatorRelationshipAssignmentRejectsTransferredTesteeBeforeWrite(t *testing.T) {
	target := makeTestee(20)
	store := uint64(8)
	target.RestoreStore(&store, 2)
	repo := &lockedRelationTesteeRepo{relationshipServiceTesteeRepo: relationshipServiceTesteeRepo{item: target}}
	relations := &relationshipServiceRelationRepo{}
	svc := &relationshipService{operatorOnly: true, operatorScope: relationReadScope{action: "update"}, relationRepo: relations, clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(10)}, testeeRepo: repo, uow: passthroughTxRunner{}}
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:*:*:*", "*")
	dto := AssignTesteeDTO{OrgID: 1, ClinicianID: 10, TesteeID: 20, RelationType: "attending"}
	if _, err := svc.AssignTestee(ctx, dto); err == nil {
		t.Fatal("transferred testee allowed")
	}
	if repo.locks != 1 || relations.saved != nil || relations.updated != nil {
		t.Fatal("scope rejection wrote relation or skipped lock")
	}
	store = 7
	target.RestoreStore(&store, 3)
	if _, err := svc.AssignTestee(ctx, dto); err != nil {
		t.Fatal(err)
	}
	if repo.locks != 2 || relations.saved == nil {
		t.Fatal("in-scope assignment not saved")
	}
	relations.saved = nil
	if _, err := svc.AssignTestee(actorctx.WithOperatorOrgID(ctx, 2), dto); err == nil {
		t.Fatal("foreign company allowed")
	}
	if relations.saved != nil || repo.locks != 2 {
		t.Fatal("foreign company accessed repository")
	}
}

type unbindScopeRepo struct {
	current *domainRelation.ClinicianTesteeRelation
	relationshipServiceRelationRepo
	item  *domainRelation.ClinicianTesteeRelation
	reads int
}

func (r *unbindScopeRepo) FindByID(context.Context, domainRelation.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	r.reads++
	return r.item, nil
}
func TestOperatorUnbindRejectsBeforeLookupAndChecksInactiveRelationScope(t *testing.T) {
	target := makeTestee(20)
	store := uint64(8)
	target.RestoreStore(&store, 2)
	repo := &lockedRelationTesteeRepo{relationshipServiceTesteeRepo: relationshipServiceTesteeRepo{item: target}}
	item := domainRelation.NewClinicianTesteeRelation(1, 10, 20, domainRelation.RelationTypeAttending, domainRelation.SourceTypeManual, nil, true, time.Now(), nil)
	item.SetID(55)
	relations := &unbindScopeRepo{item: item}
	svc := &relationshipService{operatorOnly: true, operatorScope: relationReadScope{action: "update"}, relationRepo: relations, testeeRepo: repo, uow: passthroughTxRunner{}}
	if _, err := svc.UnbindRelation(context.Background(), 55); err == nil {
		t.Fatal("unauthenticated unbind allowed")
	}
	if relations.reads != 0 {
		t.Fatal("unauthenticated request loaded relation")
	}
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:*:*:*", "*")
	if _, err := svc.UnbindRelation(ctx, 55); err == nil {
		t.Fatal("foreign store unbind allowed")
	}
	if relations.updated != nil || !item.IsActive() {
		t.Fatal("denied unbind changed relationship")
	}
	store = 7
	target.RestoreStore(&store, 3)
	if _, err := svc.UnbindRelation(ctx, 55); err != nil {
		t.Fatal(err)
	}
	if relations.updated == nil || item.IsActive() {
		t.Fatal("authorized unbind failed")
	}
	relations.updated = nil
	if _, err := svc.UnbindRelation(ctx, 55); err != nil {
		t.Fatal(err)
	}
	if relations.updated != nil {
		t.Fatal("repeat unbind wrote again")
	}
	store = 8
	target.RestoreStore(&store, 4)
	if _, err := svc.UnbindRelation(ctx, 55); err == nil {
		t.Fatal("inactive relation bypassed current scope")
	}
	if relations.updated != nil {
		t.Fatal("denied inactive relation wrote")
	}
}

func (r *unbindScopeRepo) FindByIDForUpdate(_ context.Context, org int64, testee domainTestee.ID, id domainRelation.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	if org != 1 || testee != 20 || id != 55 {
		panic("wrong relation lock identity")
	}
	if r.current != nil {
		return r.current, nil
	}
	return r.item, nil
}

func TestOperatorUnbindUsesCurrentLockedRelationInsteadOfInitialSnapshot(t *testing.T) {
	target := makeTestee(20)
	store := uint64(7)
	target.RestoreStore(&store, 1)
	repo := &lockedRelationTesteeRepo{relationshipServiceTesteeRepo: relationshipServiceTesteeRepo{item: target}}
	stale := domainRelation.NewClinicianTesteeRelation(1, 10, 20, domainRelation.RelationTypeAttending, domainRelation.SourceTypeManual, nil, true, time.Now(), nil)
	stale.SetID(55)
	current := domainRelation.NewClinicianTesteeRelation(1, 10, 20, domainRelation.RelationTypeAttending, domainRelation.SourceTypeManual, nil, false, time.Now(), nil)
	current.SetID(55)
	relations := &unbindScopeRepo{item: stale, current: current}
	svc := &relationshipService{operatorOnly: true, operatorScope: relationReadScope{action: "update"}, relationRepo: relations, testeeRepo: repo, uow: passthroughTxRunner{}}
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:*:*:*", "*")
	if _, err := svc.UnbindRelation(ctx, 55); err != nil {
		t.Fatal(err)
	}
	if relations.updated != nil || !stale.IsActive() || repo.locks != 1 {
		t.Fatal("stale relationship state overwritten")
	}
}

func (r *relationshipServiceRelationRepo) FindActivePrimaryByTesteeForUpdate(ctx context.Context, org int64, id domainTestee.ID) (*domainRelation.ClinicianTesteeRelation, error) {
	return r.FindActivePrimaryByTestee(ctx, org, id)
}
func (r *relationshipServiceRelationRepo) FindActiveByTypesForUpdate(ctx context.Context, org int64, clinician domainClinician.ID, id domainTestee.ID, types []domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	return r.FindActiveByTypes(ctx, org, clinician, id, types)
}

type currentAssignmentRepo struct {
	relationshipServiceRelationRepo
	current *domainRelation.ClinicianTesteeRelation
}

func (r *currentAssignmentRepo) FindActiveByTypesForUpdate(context.Context, int64, domainClinician.ID, domainTestee.ID, []domainRelation.RelationType) (*domainRelation.ClinicianTesteeRelation, error) {
	return r.current, nil
}
func TestOperatorAssignmentReusesRelationCommittedAfterOldSnapshot(t *testing.T) {
	target := makeTestee(20)
	store := uint64(7)
	target.RestoreStore(&store, 1)
	repo := &lockedRelationTesteeRepo{relationshipServiceTesteeRepo: relationshipServiceTesteeRepo{item: target}}
	current := domainRelation.NewClinicianTesteeRelation(1, 10, 20, domainRelation.RelationTypeAttending, domainRelation.SourceTypeManual, nil, true, time.Now(), nil)
	current.SetID(55)
	// The ordinary snapshot method returns not found; the locked method sees
	// the relation committed by an earlier writer while this request waited.
	relations := &currentAssignmentRepo{current: current}
	svc := &relationshipService{operatorOnly: true, operatorScope: relationReadScope{action: "update"}, relationRepo: relations, clinicianRepo: &relationshipServiceClinicianRepo{item: makeActiveClinician(10)}, testeeRepo: repo, uow: passthroughTxRunner{}}
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(actorctx.WithOperatorOrgID(context.Background(), 1), 9), "qs:*:*:*", "*")
	result, err := svc.AssignAttending(ctx, AssignTesteeDTO{OrgID: 1, ClinicianID: 10, TesteeID: 20})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != 55 || relations.saved != nil || relations.updated != nil {
		t.Fatal("concurrent relation was duplicated or overwritten")
	}
}
