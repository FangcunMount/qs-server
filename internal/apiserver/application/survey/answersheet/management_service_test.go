package answersheet

import (
	"context"
	actorctx "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	"math"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	questionnairedomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type managementRepoStub struct {
	findByIDFunc func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error)
	deleteFunc   func(context.Context, meta.ID) error
}

func (s *managementRepoStub) Create(context.Context, *domainanswersheet.AnswerSheet) error {
	return nil
}

func (s *managementRepoStub) Update(context.Context, *domainanswersheet.AnswerSheet) error {
	return nil
}

func (s *managementRepoStub) FindByID(ctx context.Context, id meta.ID) (*domainanswersheet.AnswerSheet, error) {
	if s.findByIDFunc != nil {
		return s.findByIDFunc(ctx, id)
	}
	return nil, nil
}

func (s *managementRepoStub) Delete(ctx context.Context, id meta.ID) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, id)
	}
	return nil
}

func TestValidateManagementListDTORejectsInvalidPaging(t *testing.T) {
	t.Parallel()

	cases := []ListAnswerSheetsDTO{
		{OrgID: 1, Page: 0, PageSize: 10},
		{OrgID: 1, Page: 1, PageSize: 0},
		{OrgID: 1, Page: 1, PageSize: 101},
	}

	for _, dto := range cases {
		if err := validateManagementListDTO(dto); err == nil {
			t.Fatalf("validateManagementListDTO(%+v) expected error", dto)
		}
	}
}

func TestValidateManagementListDTORejectsMissingOrgScope(t *testing.T) {
	t.Parallel()

	err := validateManagementListDTO(ListAnswerSheetsDTO{Page: 1, PageSize: 10})
	if err == nil || errors.ParseCoder(err).Code() != errorCode.ErrPermissionDenied {
		t.Fatalf("validateManagementListDTO() error = %v, want permission denied", err)
	}
}

func TestBuildListConditionsIncludesOptionalFilters(t *testing.T) {
	t.Parallel()

	fillerID := uint64(42)
	startTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(24 * time.Hour)
	filter := buildListFilter(ListAnswerSheetsDTO{
		OrgID:             88,
		QuestionnaireCode: "QNR-001",
		FillerID:          &fillerID,
		StartTime:         &startTime,
		EndTime:           &endTime,
	})
	if filter.OrgID != 88 {
		t.Fatalf("org_id = %d, want 88", filter.OrgID)
	}

	if got := filter.QuestionnaireCode; got != "QNR-001" {
		t.Fatalf("questionnaire_code = %v, want QNR-001", got)
	}
	if filter.FillerID == nil || *filter.FillerID != fillerID {
		t.Fatalf("filler_id = %v, want %d", filter.FillerID, fillerID)
	}
	if got := filter.StartTime; got != &startTime {
		t.Fatalf("start_time = %v, want %v", got, &startTime)
	}
	if got := filter.EndTime; got != &endTime {
		t.Fatalf("end_time = %v, want %v", got, &endTime)
	}
}

func TestManagementServiceListUsesReadModelFilter(t *testing.T) {
	t.Parallel()

	fillerID := uint64(9)
	startTime := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	reader := &answerSheetReaderStub{}
	service := &managementService{
		access: &sheetScopeStub{},
		repo:   &managementRepoStub{},
		reader: reader,
	}

	_, err := service.List(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:answersheet:collection:answersheets", "list"), ListAnswerSheetsDTO{
		OrgID:             88,
		QuestionnaireCode: "QNR-009",
		FillerID:          &fillerID,
		StartTime:         &startTime,
		Page:              1,
		PageSize:          20,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !reader.listFilter.RestrictToStoreScope || !reader.countFilter.RestrictToStoreScope {
		t.Fatal("store scope not forwarded")
	}
	if reader.listFilter.OrgID != 88 || reader.countFilter.OrgID != 88 {
		t.Fatalf("captured org filters = list:%d count:%d, want 88", reader.listFilter.OrgID, reader.countFilter.OrgID)
	}
	if reader.listFilter.QuestionnaireCode != "QNR-009" || reader.listFilter.FillerID == nil || *reader.listFilter.FillerID != fillerID {
		t.Fatalf("captured list filter = %#v", reader.listFilter)
	}
	if reader.countFilter.QuestionnaireCode != "QNR-009" || reader.countFilter.FillerID == nil || *reader.countFilter.FillerID != fillerID {
		t.Fatalf("captured count filter = %#v", reader.countFilter)
	}
}

type answerSheetReaderStub struct {
	listCalls, countCalls int
	listFilter            surveyreadmodel.AnswerSheetFilter
	countFilter           surveyreadmodel.AnswerSheetFilter
}

type answerSheetIdentityResolverStub struct {
	names map[string]string
}

func (s answerSheetIdentityResolverStub) IsEnabled() bool { return true }

func (s answerSheetIdentityResolverStub) ResolveUserNames(_ context.Context, _ []meta.ID) map[string]string {
	return s.names
}

func (s *answerSheetReaderStub) ListAnswerSheets(_ context.Context, filter surveyreadmodel.AnswerSheetFilter, _ surveyreadmodel.PageRequest) ([]surveyreadmodel.AnswerSheetSummaryRow, error) {
	s.listCalls++
	s.listFilter = filter
	return []surveyreadmodel.AnswerSheetSummaryRow{}, nil
}

func (s *answerSheetReaderStub) CountAnswerSheets(_ context.Context, filter surveyreadmodel.AnswerSheetFilter) (int64, error) {
	s.countCalls++
	s.countFilter = filter
	return 0, nil
}

func TestManagementServiceGetByIDReturnsConvertedAnswerSheet(t *testing.T) {
	t.Parallel()

	answer1, _ := domainanswersheet.NewAnswer(meta.NewCode("q1"), questionnairedomain.TypeRadio, domainanswersheet.NewOptionValue("A"), 1.5)
	answer2, _ := domainanswersheet.NewAnswer(meta.NewCode("q2"), questionnairedomain.TypeNumber, domainanswersheet.NewNumberValue(7), 2.5)
	questionnaireRef, err := domainanswersheet.NewQuestionnaireRef("QNR-001", "v1", "PHQ-9")
	if err != nil {
		t.Fatalf("NewQuestionnaireRef() error = %v", err)
	}
	subCtx, err := domainanswersheet.NewSubmissionContext(
		actor.NewFillerRef(7, actor.FillerTypeGuardian),
		actor.NewTesteeRefWithProfile(meta.FromUint64(401), 901),
		meta.FromUint64(1),
		"task-mgmt-test",
	)
	if err != nil {
		t.Fatalf("NewSubmissionContext() error = %v", err)
	}
	sheet, err := domainanswersheet.Submit(
		meta.FromUint64(12),
		questionnaireRef,
		subCtx,
		[]domainanswersheet.Answer{answer1, answer2},
		time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	service := &managementService{
		access: &sheetScopeStub{},
		repo: &managementRepoStub{
			findByIDFunc: func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error) {
				return sheet, nil
			},
		},
		identityResolver: answerSheetIdentityResolverStub{names: map[string]string{"7": "填写人七号"}},
	}

	result, err := service.GetByID(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if result.ID != 12 || result.QuestionnaireCode != "QNR-001" || result.QuestionnaireVer != "v1" {
		t.Fatalf("unexpected identity fields: %+v", result)
	}
	if result.OrgID != 1 {
		t.Fatalf("OrgID = %d, want 1", result.OrgID)
	}
	if result.FillerID != 7 || len(result.Answers) != 2 {
		t.Fatalf("unexpected filler/answers: %+v", result)
	}
	if result.FillerName != "填写人七号" {
		t.Fatalf("FillerName = %q, want IAM nickname", result.FillerName)
	}
	if result.Answers[0].QuestionCode != "q1" || result.Answers[1].QuestionCode != "q2" {
		t.Fatalf("unexpected answers: %+v", result.Answers)
	}
	t.Run("business organization ownership remains enforced without authorization partition context", func(t *testing.T) {
		questions := &versionQuestionReader{}
		service.questions = questions
		if _, err := service.GetByIDInOrg(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:answersheet:collection:answersheets", "read"), 1, 12); err != nil {
			t.Fatalf("GetByIDInOrg same org returned error: %v", err)
		}
		if questions.calls != 1 || questions.version != "v1" {
			t.Fatal("missing recorded-version display read")
		}
		if _, err := service.GetByIDInOrg(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:answersheet:collection:answersheets", "read"), 2, 12); errors.ParseCoder(err).Code() != errorCode.ErrAnswerSheetNotFound {
			t.Fatalf("GetByIDInOrg cross org error = %v, want not found", err)
		}
		if _, err := service.GetByIDInOrg(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), "qs:answersheet:collection:answersheets", "read"), 0, 12); errors.ParseCoder(err).Code() != errorCode.ErrPermissionDenied {
			t.Fatalf("GetByIDInOrg missing org error = %v, want permission denied", err)
		}
		if _, err := service.GetByIDInOrg(context.Background(), 1, 12); err == nil {
			t.Fatal("missing action accepted")
		}
		service.access = &sheetScopeStub{err: errors.New("outside store")}
		if _, err := service.GetByIDInOrg(authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), appauthz.AnswerSheetResource, "read"), 1, 12); err == nil {
			t.Fatal("outside store accepted")
		}
		if questions.calls != 1 {
			t.Fatal("question content queried before scope or action authorization")
		}
	})
}

func TestManagementServiceDeleteDelegatesToRepository(t *testing.T) {
	t.Parallel()

	var deletedID meta.ID
	service := &managementService{
		access: &sheetScopeStub{},
		repo: &managementRepoStub{
			findByIDFunc: func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error) {
				return &domainanswersheet.AnswerSheet{}, nil
			},
			deleteFunc: func(_ context.Context, id meta.ID) error {
				deletedID = id
				return nil
			},
		},
	}

	if err := service.Delete(context.Background(), 9); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if deletedID != meta.FromUint64(9) {
		t.Fatalf("deletedID = %d, want 9", deletedID)
	}
}

func TestAnswerSheetIDConvertersRejectOverflow(t *testing.T) {
	t.Parallel()

	if _, err := answerSheetIDFromUint64("answersheet_id", math.MaxUint64); err == nil {
		t.Fatal("answerSheetIDFromUint64 expected overflow error")
	}
	if _, err := fillerUserIDFromUint64("filler_id", math.MaxUint64); err == nil {
		t.Fatal("fillerUserIDFromUint64 expected overflow error")
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("mustUint64FromInt64 expected panic on negative input")
		}
	}()
	_ = mustUint64FromInt64("filler_id", -1)
}

func TestManagementServiceDeleteWrapsMissingAnswerSheet(t *testing.T) {
	t.Parallel()

	service := &managementService{
		access: &sheetScopeStub{},
		repo: &managementRepoStub{
			findByIDFunc: func(context.Context, meta.ID) (*domainanswersheet.AnswerSheet, error) {
				return nil, errors.WithCode(errorCode.ErrAnswerSheetNotFound, "missing")
			},
		},
	}

	err := service.Delete(context.Background(), 7)
	if err == nil {
		t.Fatal("Delete expected error")
	}
	if code := errors.ParseCoder(err).Code(); code != errorCode.ErrAnswerSheetNotFound {
		t.Fatalf("error code = %d, want %d", code, errorCode.ErrAnswerSheetNotFound)
	}
}

// Retiring an Operator must not make historical role labels sufficient to read results.
func TestManagementListRejectsRevokedOperatorBeforeReadingResults(t *testing.T) {
	for _, ctx := range []context.Context{
		context.Background(),
		appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{DirectRoles: []string{"qs:result_reviewer"}, AuthzVersion: 42}),
	} {
		reader := &answerSheetReaderStub{}
		service := &managementService{
			access: &sheetScopeStub{}, reader: reader}
		result, err := service.List(ctx, ListAnswerSheetsDTO{OrgID: 88, Page: 1, PageSize: 20})
		if result != nil || err == nil || errors.ParseCoder(err).Code() != errorCode.ErrPermissionDenied {
			t.Fatalf("revoked Operator result=%v error=%v", result, err)
		}
		if reader.listFilter.OrgID != 0 || reader.countFilter.OrgID != 0 {
			t.Fatal("results or count read before authorization")
		}
	}
}

type sheetScopeStub struct {
	err    error
	action string
}

func (s *sheetScopeStub) ValidateTesteeStoreAccess(_ context.Context, _, _ int64, _ uint64, resource, action string) error {
	if resource != appauthz.AnswerSheetResource {
		return errors.New("wrong resource")
	}
	s.action = action
	return s.err
}
func (s *sheetScopeStub) ListStoreScopedTesteeIDs(_ context.Context, _, _ int64, resource, action string) ([]uint64, error) {
	if resource != appauthz.AnswerSheetResource {
		return nil, errors.New("wrong resource")
	}
	s.action = action
	return []uint64{8}, s.err
}
func TestManagementListRejectsScopeBeforeQuery(t *testing.T) {
	access := &sheetScopeStub{err: errors.New("outside store scope")}
	reader := &answerSheetReaderStub{}
	service := &managementService{access: access, reader: reader}
	ctx := authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), appauthz.AnswerSheetResource, "list")
	_, err := service.List(ctx, ListAnswerSheetsDTO{OrgID: 1, Page: 1, PageSize: 10})
	if err == nil {
		t.Fatal("scope denial ignored")
	}
	if reader.listCalls != 0 || reader.countCalls != 0 {
		t.Fatal("denied scope queried answer sheets")
	}
	if access.action != "list" {
		t.Fatal("incorrect action")
	}
}

func TestManagementListTesteeFilterIsIndependentOfFillerAndChecksScope(t *testing.T) {
	testeeID, fillerID := uint64(401), uint64(7)
	for _, denied := range []bool{false, true} {
		reader := &answerSheetReaderStub{}
		access := &sheetScopeStub{}
		if denied {
			access.err = errors.New("outside store")
		}
		service := &managementService{access: access, reader: reader}
		ctx := authztest.WithPermission(actorctx.WithGrantingUserID(context.Background(), 9), appauthz.AnswerSheetResource, "list")
		_, err := service.List(ctx, ListAnswerSheetsDTO{OrgID: 1, TesteeID: &testeeID, FillerID: &fillerID, Page: 1, PageSize: 10})
		if denied {
			if err == nil || reader.listCalls != 0 || reader.countCalls != 0 {
				t.Fatal("scope bypass")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, filter := range []surveyreadmodel.AnswerSheetFilter{reader.listFilter, reader.countFilter} {
			if !filter.RestrictToStoreScope || len(filter.StoreScopedTesteeIDs) != 1 || filter.StoreScopedTesteeIDs[0] != testeeID || filter.FillerID == nil || *filter.FillerID != fillerID {
				t.Fatalf("incorrect list/count filter: %+v", filter)
			}
		}
	}
}
