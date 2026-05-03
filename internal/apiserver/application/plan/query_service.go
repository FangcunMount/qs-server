package plan

import (
	"context"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 计划查询服务实现
// 行为者：所有用户
type queryService struct {
	planReader   planreadmodel.PlanReader
	taskReader   planreadmodel.TaskReader
	scaleCatalog ScaleCatalog
}

// NewQueryService 创建计划查询服务
func NewQueryService(
	planReader planreadmodel.PlanReader,
	taskReader planreadmodel.TaskReader,
	scaleCatalog ScaleCatalog,
) PlanQueryService {
	return &queryService{
		planReader:   planReader,
		taskReader:   taskReader,
		scaleCatalog: scaleCatalog,
	}
}

// GetPlan 根据ID获取计划
func (s *queryService) GetPlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	if s.planReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "plan read model is not configured")
	}

	row, err := s.planReader.GetPlan(ctx, orgID, id.Uint64())
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}
	result := toPlanResultFromRow(*row)
	result.ScaleTitle = s.resolveScaleTitle(ctx, result.ScaleCode)
	return result, nil
}

// ListPlans 查询计划列表
func (s *queryService) ListPlans(ctx context.Context, dto ListPlansDTO) (*PlanListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	if dto.PageSize > 100 {
		dto.PageSize = 100 // 限制最大每页数量
	}

	if s.planReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "plan read model is not configured")
	}

	page, err := s.planReader.ListPlans(ctx, planreadmodel.PlanFilter{
		OrgID:     dto.OrgID,
		ScaleCode: dto.ScaleCode,
		Status:    dto.Status,
	}, planreadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询计划列表失败")
	}
	scaleTitles := s.resolveScaleTitles(ctx, collectPlanScaleCodesFromRows(page.Items))
	items := make([]*PlanResult, 0, len(page.Items))
	for _, row := range page.Items {
		item := toPlanResultFromRow(row)
		item.ScaleTitle = scaleTitles[item.ScaleCode]
		items = append(items, item)
	}
	return &PlanListResult{Items: items, Total: page.Total, Page: dto.Page, PageSize: dto.PageSize}, nil
}

// GetTask 根据ID获取任务
func (s *queryService) GetTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	// 1. 转换参数
	id, err := toTaskID(taskID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	if s.taskReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "task read model is not configured")
	}

	row, err := s.taskReader.GetTask(ctx, orgID, id.Uint64())
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}
	result := toTaskResultFromRow(*row)
	result.ScaleTitle = s.resolveScaleTitle(ctx, result.ScaleCode)
	return result, nil
}

// ListTasks 查询任务列表
func (s *queryService) ListTasks(ctx context.Context, dto ListTasksDTO) (*TaskListResult, error) {
	// 1. 验证分页参数
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	if dto.PageSize > 100 {
		dto.PageSize = 100 // 限制最大每页数量
	}

	// 2. 转换查询条件
	var planID *domainPlan.AssessmentPlanID
	if dto.PlanID != "" {
		id, err := toPlanID(dto.PlanID)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
		}
		planID = &id
	}

	var testeeID *testee.ID
	if dto.TesteeID != "" {
		id, err := toTesteeID(dto.TesteeID)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
		}
		testeeID = &id
	}

	var status *domainPlan.TaskStatus
	if dto.Status != "" {
		statusVal := domainPlan.TaskStatus(dto.Status)
		if !statusVal.IsValid() {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务状态: %s", dto.Status)
		}
		status = &statusVal
	}

	if s.taskReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "task read model is not configured")
	}
	filter := planreadmodel.TaskFilter{
		OrgID:                 dto.OrgID,
		RestrictToAccessScope: dto.RestrictToAccessScope,
	}
	if planID != nil {
		rawPlanID := planID.Uint64()
		filter.PlanID = &rawPlanID
	}
	if testeeID != nil {
		rawTesteeID := testeeID.Uint64()
		filter.TesteeID = &rawTesteeID
	}
	if status != nil {
		rawStatus := status.String()
		filter.Status = &rawStatus
	}
	if dto.RestrictToAccessScope {
		filter.AccessibleTesteeIDs = make([]uint64, 0, len(dto.AccessibleTesteeIDs))
		for _, rawID := range dto.AccessibleTesteeIDs {
			id, err := toTesteeID(rawID)
			if err != nil {
				return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
			}
			filter.AccessibleTesteeIDs = append(filter.AccessibleTesteeIDs, id.Uint64())
		}
	}
	page, err := s.taskReader.ListTasks(ctx, filter, planreadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务列表失败")
	}
	return &TaskListResult{
		Items:    s.toTaskResultsWithScaleTitlesFromRows(ctx, page.Items),
		Total:    page.Total,
		Page:     dto.Page,
		PageSize: dto.PageSize,
	}, nil
}

// ListTaskWindow 查询计划任务窗口。
func (s *queryService) ListTaskWindow(ctx context.Context, dto ListTaskWindowDTO) (*TaskWindowResult, error) {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	if dto.PageSize > 100 {
		dto.PageSize = 100
	}

	planID, err := toPlanID(dto.PlanID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	var status *domainPlan.TaskStatus
	if rawStatus := strings.TrimSpace(dto.Status); rawStatus != "" {
		statusVal := domainPlan.TaskStatus(rawStatus)
		if !statusVal.IsValid() {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务状态: %s", dto.Status)
		}
		status = &statusVal
	}

	var plannedBefore *time.Time
	if rawBefore := strings.TrimSpace(dto.PlannedBefore); rawBefore != "" {
		parsed, err := parseTime(rawBefore)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的 planned_before: %v", err)
		}
		plannedBefore = &parsed
	}

	testeeIDs := make([]testee.ID, 0, len(dto.TesteeIDs))
	for _, rawID := range dto.TesteeIDs {
		id, err := toTesteeID(rawID)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
		}
		testeeIDs = append(testeeIDs, id)
	}

	if s.taskReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "task read model is not configured")
	}
	filter := planreadmodel.TaskWindowFilter{
		OrgID:         dto.OrgID,
		PlanID:        planID.Uint64(),
		PlannedBefore: plannedBefore,
	}
	if status != nil {
		rawStatus := status.String()
		filter.Status = &rawStatus
	}
	if len(testeeIDs) > 0 {
		filter.TesteeIDs = make([]uint64, 0, len(testeeIDs))
		for _, id := range testeeIDs {
			filter.TesteeIDs = append(filter.TesteeIDs, id.Uint64())
		}
	}
	window, err := s.taskReader.ListTaskWindow(ctx, filter, planreadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务窗口失败")
	}
	return &TaskWindowResult{
		Items:    s.toTaskResultsWithScaleTitlesFromRows(ctx, window.Items),
		Page:     dto.Page,
		PageSize: dto.PageSize,
		HasMore:  window.HasMore,
	}, nil
}

// ListTasksByPlan 查询计划下的所有任务
func (s *queryService) ListTasksByPlan(ctx context.Context, orgID int64, planID string) ([]*TaskResult, error) {
	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}
	if err := s.ensurePlanInOrg(ctx, orgID, id); err != nil {
		return nil, err
	}

	if s.taskReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "task read model is not configured")
	}
	rows, err := s.taskReader.ListTasksByPlanID(ctx, id.Uint64())
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务失败")
	}
	return s.toTaskResultsWithScaleTitlesFromRows(ctx, rows), nil
}

// ListTasksByPlanInScope 查询计划下指定可访问范围内的任务。
func (s *queryService) ListTasksByPlanInScope(ctx context.Context, orgID int64, planID string, accessibleTesteeIDs []string) ([]*TaskResult, error) {
	id, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}
	if err := s.ensurePlanInOrg(ctx, orgID, id); err != nil {
		return nil, err
	}

	testeeIDs := make([]testee.ID, 0, len(accessibleTesteeIDs))
	for _, rawID := range accessibleTesteeIDs {
		testeeID, err := toTesteeID(rawID)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
		}
		testeeIDs = append(testeeIDs, testeeID)
	}

	if s.taskReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "task read model is not configured")
	}
	rawIDs := make([]uint64, 0, len(testeeIDs))
	for _, id := range testeeIDs {
		rawIDs = append(rawIDs, id.Uint64())
	}
	rows, err := s.taskReader.ListTasksByPlanIDAndTesteeIDs(ctx, id.Uint64(), rawIDs)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务失败")
	}
	return s.toTaskResultsWithScaleTitlesFromRows(ctx, rows), nil
}

func (s *queryService) ensurePlanInOrg(ctx context.Context, orgID int64, planID domainPlan.AssessmentPlanID) error {
	if s.planReader == nil {
		return errors.WithCode(errorCode.ErrModuleInitializationFailed, "plan read model is not configured")
	}
	if _, err := s.planReader.GetPlan(ctx, orgID, planID.Uint64()); err != nil {
		return errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}
	return nil
}

// ListTasksByTestee 查询受试者的所有任务
func (s *queryService) ListTasksByTestee(ctx context.Context, testeeID string) ([]*TaskResult, error) {
	// 1. 转换参数
	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	if s.taskReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "task read model is not configured")
	}
	rows, err := s.taskReader.ListTasksByTesteeID(ctx, testeeIDDomain.Uint64())
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务失败")
	}
	return s.toTaskResultsWithScaleTitlesFromRows(ctx, rows), nil
}

// ListPlansByTestee 查询受试者参与的所有计划
func (s *queryService) ListPlansByTestee(ctx context.Context, testeeID string) ([]*PlanResult, error) {
	// 1. 转换参数
	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	if s.planReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "plan read model is not configured")
	}
	rows, err := s.planReader.ListPlansByTesteeID(ctx, testeeIDDomain.Uint64())
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询计划失败")
	}
	scaleTitles := s.resolveScaleTitles(ctx, collectPlanScaleCodesFromRows(rows))
	results := make([]*PlanResult, 0, len(rows))
	for _, row := range rows {
		result := toPlanResultFromRow(row)
		result.ScaleTitle = scaleTitles[result.ScaleCode]
		results = append(results, result)
	}
	return results, nil
}

// ListTasksByTesteeAndPlan 查询受试者在某个计划下的所有任务
func (s *queryService) ListTasksByTesteeAndPlan(ctx context.Context, testeeID string, planID string) ([]*TaskResult, error) {
	// 1. 转换参数
	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	planIDDomain, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	if s.taskReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "task read model is not configured")
	}
	rows, err := s.taskReader.ListTasksByTesteeIDAndPlanID(ctx, testeeIDDomain.Uint64(), planIDDomain.Uint64())
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询任务失败")
	}
	return s.toTaskResultsWithScaleTitlesFromRows(ctx, rows), nil
}

func (s *queryService) toTaskResultsWithScaleTitlesFromRows(ctx context.Context, rows []planreadmodel.TaskRow) []*TaskResult {
	results := toTaskResultsFromRows(rows)
	if len(results) == 0 {
		return results
	}
	scaleTitles := s.resolveScaleTitles(ctx, collectTaskScaleCodesFromRows(rows))
	for idx, row := range rows {
		results[idx].ScaleTitle = scaleTitles[row.ScaleCode]
	}
	return results
}

func (s *queryService) resolveScaleTitle(ctx context.Context, scaleCode string) string {
	if scaleCode == "" {
		return ""
	}
	return s.resolveScaleTitles(ctx, []string{scaleCode})[scaleCode]
}

func (s *queryService) resolveScaleTitles(ctx context.Context, scaleCodes []string) map[string]string {
	if s == nil || s.scaleCatalog == nil || len(scaleCodes) == 0 {
		return make(map[string]string, len(scaleCodes))
	}
	return s.scaleCatalog.ResolveTitles(ctx, scaleCodes)
}

func collectPlanScaleCodesFromRows(rows []planreadmodel.PlanRow) []string {
	codes := make([]string, 0, len(rows))
	for _, item := range rows {
		codes = append(codes, item.ScaleCode)
	}
	return codes
}

func collectTaskScaleCodesFromRows(rows []planreadmodel.TaskRow) []string {
	codes := make([]string, 0, len(rows))
	for _, item := range rows {
		codes = append(codes, item.ScaleCode)
	}
	return codes
}
