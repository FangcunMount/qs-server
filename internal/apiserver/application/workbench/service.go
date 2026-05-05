package workbench

import (
	"context"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type operatorByUserQuery interface {
	GetByUser(ctx context.Context, orgID int64, userID int64) (*operatorApp.OperatorResult, error)
}

type clinicianByOperatorQuery interface {
	GetByOperator(ctx context.Context, orgID int64, operatorID uint64) (*clinicianApp.ClinicianResult, error)
}

type clinicianAssignmentReader interface {
	ListAssignedTesteeIDs(ctx context.Context, orgID int64, clinicianID uint64) ([]uint64, error)
}

type assignmentHydrator interface {
	ListActiveTesteeRelationsByTesteeIDs(ctx context.Context, orgID int64, testeeIDs []uint64, relationTypes []string) ([]actorreadmodel.TesteeRelationRow, error)
}

type testeeReader interface {
	actorreadmodel.TesteeHydrator
	ListTestees(ctx context.Context, filter actorreadmodel.TesteeFilter) ([]actorreadmodel.TesteeRow, error)
	CountTestees(ctx context.Context, filter actorreadmodel.TesteeFilter) (int64, error)
}

type service struct {
	operatorQuery       operatorByUserQuery
	clinicianQuery      clinicianByOperatorQuery
	relationshipService clinicianAssignmentReader
	assignmentHydrator  assignmentHydrator
	testeeReader        testeeReader
	latestRiskReader    evaluationreadmodel.LatestRiskReader
	followUpQueueReader planreadmodel.FollowUpQueueReader
}

func NewService(
	operatorQuery operatorByUserQuery,
	clinicianQuery clinicianByOperatorQuery,
	relationshipService clinicianAssignmentReader,
	assignmentHydrator assignmentHydrator,
	testeeReader testeeReader,
	latestRiskReader evaluationreadmodel.LatestRiskReader,
	followUpQueueReader planreadmodel.FollowUpQueueReader,
) Service {
	return &service{
		operatorQuery:       operatorQuery,
		clinicianQuery:      clinicianQuery,
		relationshipService: relationshipService,
		assignmentHydrator:  assignmentHydrator,
		testeeReader:        testeeReader,
		latestRiskReader:    latestRiskReader,
		followUpQueueReader: followUpQueueReader,
	}
}

func (s *service) GetSummary(ctx context.Context, scope Scope) (*SummaryResult, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	resolved, ok, err := s.resolveScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	if !ok || resolved.isEmpty() {
		return &SummaryResult{}, nil
	}

	highRiskPage, err := s.latestRiskReader.ListLatestRiskQueue(ctx, latestRiskQueueFilter(resolved), evaluationreadmodel.PageRequest{Page: 1, PageSize: 1})
	if err != nil {
		return nil, errors.Wrap(err, "failed to count high risk queue")
	}
	followUpPage, err := s.followUpQueueReader.ListFollowUpQueueTasks(ctx, planreadmodel.FollowUpQueueFilter{
		OrgID:               resolved.OrgID,
		TesteeIDs:           resolved.TesteeIDs,
		RestrictToTesteeIDs: resolved.RestrictToTesteeIDs,
	}, planreadmodel.PageRequest{Page: 1, PageSize: 1})
	if err != nil {
		return nil, errors.Wrap(err, "failed to count follow-up queue")
	}
	keyFocusCount, err := s.testeeReader.CountTestees(ctx, s.keyFocusFilter(resolved, 0, 0))
	if err != nil {
		return nil, errors.Wrap(err, "failed to count key focus queue")
	}

	return &SummaryResult{
		Counts: QueueCounts{
			HighRisk: highRiskPage.Total,
			FollowUp: followUpPage.Total,
			KeyFocus: keyFocusCount,
		},
	}, nil
}

func (s *service) ListQueue(ctx context.Context, dto ListQueueDTO) (*QueuePage, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	queueType, err := normalizeQueueType(dto.QueueType)
	if err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(dto.Page, dto.PageSize)
	resolved, ok, err := s.resolveScope(ctx, dto.Scope)
	if err != nil {
		return nil, err
	}
	if !ok || resolved.isEmpty() {
		return emptyQueuePage(queueType, page, pageSize), nil
	}

	switch queueType {
	case QueueTypeHighRisk:
		return s.listHighRiskQueue(ctx, resolved, page, pageSize)
	case QueueTypeFollowUp:
		return s.listFollowUpQueue(ctx, resolved, page, pageSize)
	case QueueTypeKeyFocus:
		return s.listKeyFocusQueue(ctx, resolved, page, pageSize)
	default:
		return nil, errors.WithCode(code.ErrInvalidArgument, "unsupported workbench queue type")
	}
}

func (s *service) listHighRiskQueue(ctx context.Context, resolved resolvedScope, page, pageSize int) (*QueuePage, error) {
	riskPage, err := s.latestRiskReader.ListLatestRiskQueue(ctx, latestRiskQueueFilter(resolved), evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, err
	}
	testeesByID, err := s.hydrateTestees(ctx, resolved.OrgID, latestRiskTesteeIDs(riskPage.Items))
	if err != nil {
		return nil, err
	}

	items := make([]QueueItem, 0, len(riskPage.Items))
	for _, row := range riskPage.Items {
		testee, ok := testeesByID[row.TesteeID]
		if !ok {
			continue
		}
		reasonAt := row.OccurredAt
		items = append(items, QueueItem{
			Testee:     testee,
			ReasonCode: latestRiskReasonCode(row.RiskLevel),
			Reason:     latestRiskReason(row.RiskLevel),
			ReasonAt:   &reasonAt,
			RiskLevel:  row.RiskLevel,
		})
	}
	if resolved.IncludeAssignments {
		items, err = s.withAssignments(ctx, resolved.OrgID, items)
		if err != nil {
			return nil, err
		}
	}

	return queuePage(QueueTypeHighRisk, items, riskPage.Total, page, pageSize), nil
}

func (s *service) listFollowUpQueue(ctx context.Context, resolved resolvedScope, page, pageSize int) (*QueuePage, error) {
	taskPage, err := s.followUpQueueReader.ListFollowUpQueueTasks(ctx, planreadmodel.FollowUpQueueFilter{
		OrgID:               resolved.OrgID,
		TesteeIDs:           resolved.TesteeIDs,
		RestrictToTesteeIDs: resolved.RestrictToTesteeIDs,
	}, planreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list follow-up queue")
	}
	testeesByID, err := s.hydrateTestees(ctx, resolved.OrgID, taskTesteeIDs(taskPage.Items))
	if err != nil {
		return nil, err
	}

	items := make([]QueueItem, 0, len(taskPage.Items))
	for _, task := range taskPage.Items {
		testee, ok := testeesByID[task.TesteeID]
		if !ok {
			continue
		}
		reasonAt := followUpReasonAt(task)
		items = append(items, QueueItem{
			Testee:     testee,
			ReasonCode: followUpReasonCode(task.Status),
			Reason:     followUpReason(task.Status),
			ReasonAt:   reasonAt,
			Task:       taskSummary(task),
		})
	}
	if resolved.IncludeAssignments {
		items, err = s.withAssignments(ctx, resolved.OrgID, items)
		if err != nil {
			return nil, err
		}
	}

	return queuePage(QueueTypeFollowUp, items, taskPage.Total, page, pageSize), nil
}

func (s *service) listKeyFocusQueue(ctx context.Context, resolved resolvedScope, page, pageSize int) (*QueuePage, error) {
	filter := s.keyFocusFilter(resolved, (page-1)*pageSize, pageSize)
	rows, err := s.testeeReader.ListTestees(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list key focus queue")
	}
	total, err := s.testeeReader.CountTestees(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count key focus queue")
	}

	items := make([]QueueItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, QueueItem{
			Testee:     testeeFromRow(row),
			ReasonCode: "key_focus",
			Reason:     "重点关注",
		})
	}
	if resolved.IncludeAssignments {
		items, err = s.withAssignments(ctx, resolved.OrgID, items)
		if err != nil {
			return nil, err
		}
	}
	return queuePage(QueueTypeKeyFocus, items, total, page, pageSize), nil
}

type resolvedScope struct {
	OrgID               int64
	TesteeIDs           []uint64
	RestrictToTesteeIDs bool
	IncludeAssignments  bool
}

func (s resolvedScope) isEmpty() bool {
	return s.RestrictToTesteeIDs && len(s.TesteeIDs) == 0
}

func (s *service) resolveScope(ctx context.Context, scope Scope) (resolvedScope, bool, error) {
	switch scope.Kind {
	case "", ScopeKindClinicianMe:
		ids, ok, err := s.assignedTesteeIDs(ctx, scope)
		return resolvedScope{
			OrgID:               scope.OrgID,
			TesteeIDs:           ids,
			RestrictToTesteeIDs: true,
		}, ok, err
	case ScopeKindOrgAdmin:
		if scope.OrgID <= 0 {
			return resolvedScope{}, false, nil
		}
		if scope.ClinicianID == nil {
			return resolvedScope{OrgID: scope.OrgID, IncludeAssignments: true}, true, nil
		}
		ids, err := s.relationshipService.ListAssignedTesteeIDs(ctx, scope.OrgID, *scope.ClinicianID)
		if err != nil {
			return resolvedScope{}, false, errors.Wrap(err, "failed to list clinician assigned testees")
		}
		return resolvedScope{
			OrgID:               scope.OrgID,
			TesteeIDs:           uniqueUint64(ids),
			RestrictToTesteeIDs: true,
			IncludeAssignments:  true,
		}, true, nil
	default:
		return resolvedScope{}, false, errors.WithCode(code.ErrInvalidArgument, "unsupported workbench scope")
	}
}

func (s *service) assignedTesteeIDs(ctx context.Context, scope Scope) ([]uint64, bool, error) {
	if scope.OrgID <= 0 || scope.OperatorUserID <= 0 {
		return nil, false, nil
	}
	operatorItem, err := s.operatorQuery.GetByUser(ctx, scope.OrgID, scope.OperatorUserID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, false, nil
		}
		return nil, false, errors.Wrap(err, "failed to find current operator")
	}
	if operatorItem == nil || !operatorItem.IsActive {
		return nil, false, nil
	}
	clinicianItem, err := s.clinicianQuery.GetByOperator(ctx, scope.OrgID, operatorItem.ID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, false, nil
		}
		return nil, false, errors.Wrap(err, "failed to find current clinician")
	}
	if clinicianItem == nil || !clinicianItem.IsActive {
		return nil, false, nil
	}
	ids, err := s.relationshipService.ListAssignedTesteeIDs(ctx, scope.OrgID, clinicianItem.ID)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to list assigned testees")
	}
	return uniqueUint64(ids), true, nil
}

func (s *service) hydrateTestees(ctx context.Context, orgID int64, ids []uint64) (map[uint64]Testee, error) {
	rows, err := s.testeeReader.ListTesteesByIDs(ctx, orgID, ids)
	if err != nil {
		return nil, errors.Wrap(err, "failed to hydrate queue testees")
	}
	result := make(map[uint64]Testee, len(rows))
	for _, row := range rows {
		result[row.ID] = testeeFromRow(row)
	}
	return result, nil
}

func (s *service) keyFocusFilter(scope resolvedScope, offset, limit int) actorreadmodel.TesteeFilter {
	keyFocus := true
	filter := actorreadmodel.TesteeFilter{
		OrgID:                 scope.OrgID,
		KeyFocus:              &keyFocus,
		AccessibleTesteeIDs:   scope.TesteeIDs,
		RestrictToAccessScope: scope.RestrictToTesteeIDs,
		Offset:                offset,
		Limit:                 limit,
	}
	return filter
}

func latestRiskQueueFilter(scope resolvedScope) evaluationreadmodel.LatestRiskQueueFilter {
	return evaluationreadmodel.LatestRiskQueueFilter{
		OrgID:               scope.OrgID,
		TesteeIDs:           scope.TesteeIDs,
		RestrictToTesteeIDs: scope.RestrictToTesteeIDs,
		RiskLevels:          []string{"high", "severe"},
	}
}

func (s *service) withAssignments(ctx context.Context, orgID int64, items []QueueItem) ([]QueueItem, error) {
	testeeIDs := queueItemTesteeIDs(items)
	relationRows, err := s.assignmentHydrator.ListActiveTesteeRelationsByTesteeIDs(
		ctx,
		orgID,
		testeeIDs,
		relationTypesToStrings(domainRelation.AccessGrantRelationTypes()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to hydrate queue clinician assignments")
	}
	assignmentsByTesteeID := groupAssignmentsByTesteeID(relationRows)
	result := make([]QueueItem, 0, len(items))
	for _, item := range items {
		assignments := assignmentsByTesteeID[item.Testee.ID]
		isUnassigned := len(assignments) == 0
		item.AssignedClinicians = assignments
		item.PrimaryClinician = primaryAssignment(assignments)
		item.IsUnassigned = &isUnassigned
		result = append(result, item)
	}
	return result, nil
}

func groupAssignmentsByTesteeID(rows []actorreadmodel.TesteeRelationRow) map[uint64][]ClinicianAssignment {
	result := make(map[uint64][]ClinicianAssignment)
	for _, row := range rows {
		result[row.Relation.TesteeID] = append(result[row.Relation.TesteeID], clinicianAssignmentFromRow(row))
	}
	return result
}

func clinicianAssignmentFromRow(row actorreadmodel.TesteeRelationRow) ClinicianAssignment {
	return ClinicianAssignment{
		ID:            row.Clinician.ID,
		OrgID:         row.Clinician.OrgID,
		OperatorID:    row.Clinician.OperatorID,
		Name:          row.Clinician.Name,
		Department:    row.Clinician.Department,
		Title:         row.Clinician.Title,
		ClinicianType: row.Clinician.ClinicianType,
		RelationType:  row.Relation.RelationType,
		BoundAt:       row.Relation.BoundAt,
	}
}

func primaryAssignment(items []ClinicianAssignment) *ClinicianAssignment {
	for i := range items {
		if items[i].RelationType == string(domainRelation.RelationTypePrimary) {
			return &items[i]
		}
	}
	return nil
}

func relationTypesToStrings(types []domainRelation.RelationType) []string {
	result := make([]string, 0, len(types))
	for _, relationType := range types {
		result = append(result, relationType.String())
	}
	return result
}

func (s *service) ensureConfigured() error {
	if s.operatorQuery == nil ||
		s.clinicianQuery == nil ||
		s.relationshipService == nil ||
		s.assignmentHydrator == nil ||
		s.testeeReader == nil ||
		s.latestRiskReader == nil ||
		s.followUpQueueReader == nil {
		return errors.WithCode(code.ErrInternalServerError, "clinician workbench service is not configured")
	}
	return nil
}

func normalizeQueueType(raw QueueType) (QueueType, error) {
	switch QueueType(strings.TrimSpace(string(raw))) {
	case QueueTypeHighRisk:
		return QueueTypeHighRisk, nil
	case QueueTypeFollowUp:
		return QueueTypeFollowUp, nil
	case QueueTypeKeyFocus:
		return QueueTypeKeyFocus, nil
	default:
		return "", errors.WithCode(code.ErrInvalidArgument, "unsupported workbench queue type")
	}
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func queuePage(queueType QueueType, items []QueueItem, total int64, page, pageSize int) *QueuePage {
	totalPages := 0
	if pageSize > 0 {
		totalPages = int(total) / pageSize
		if int(total)%pageSize > 0 {
			totalPages++
		}
	}
	return &QueuePage{
		QueueType:  queueType,
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

func emptyQueuePage(queueType QueueType, page, pageSize int) *QueuePage {
	return queuePage(queueType, []QueueItem{}, 0, page, pageSize)
}

func latestRiskReasonCode(riskLevel string) string {
	if riskLevel == "severe" {
		return "latest_risk_severe"
	}
	return "latest_risk_high"
}

func latestRiskReason(riskLevel string) string {
	if riskLevel == "severe" {
		return "最近测评严重风险"
	}
	return "最近测评高风险"
}

func followUpReasonCode(status string) string {
	if status == "expired" {
		return "follow_up_expired"
	}
	return "follow_up_opened"
}

func followUpReason(status string) string {
	if status == "expired" {
		return "复诊任务已逾期"
	}
	return "复诊任务待完成"
}

func followUpReasonAt(task planreadmodel.TaskRow) *time.Time {
	if task.Status == "expired" && task.ExpireAt != nil {
		return task.ExpireAt
	}
	if task.OpenAt != nil {
		return task.OpenAt
	}
	return &task.PlannedAt
}

func taskSummary(task planreadmodel.TaskRow) *TaskSummary {
	return &TaskSummary{
		TaskID:    task.ID,
		PlanID:    task.PlanID,
		Status:    task.Status,
		PlannedAt: task.PlannedAt,
		OpenAt:    task.OpenAt,
		ExpireAt:  task.ExpireAt,
		ScaleCode: task.ScaleCode,
		EntryURL:  task.EntryURL,
	}
}

func testeeFromRow(row actorreadmodel.TesteeRow) Testee {
	return Testee{
		ID:               row.ID,
		OrgID:            row.OrgID,
		ProfileID:        row.ProfileID,
		Name:             row.Name,
		Gender:           row.Gender,
		Birthday:         row.Birthday,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		Tags:             append([]string(nil), row.Tags...),
		Source:           row.Source,
		IsKeyFocus:       row.IsKeyFocus,
		LastAssessmentAt: row.LastAssessmentAt,
		TotalAssessments: row.TotalAssessments,
		LastRiskLevel:    row.LastRiskLevel,
	}
}

func latestRiskTesteeIDs(rows []evaluationreadmodel.LatestRiskRow) []uint64 {
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TesteeID)
	}
	return ids
}

func taskTesteeIDs(rows []planreadmodel.TaskRow) []uint64 {
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TesteeID)
	}
	return ids
}

func queueItemTesteeIDs(items []QueueItem) []uint64 {
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Testee.ID)
	}
	return ids
}

func uniqueUint64(items []uint64) []uint64 {
	if len(items) == 0 {
		return []uint64{}
	}
	seen := make(map[uint64]struct{}, len(items))
	result := make([]uint64, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
