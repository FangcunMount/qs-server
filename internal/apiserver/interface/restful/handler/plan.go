package handler

import (
	"bytes"
	"io"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
)

// PlanHandler 计划处理器
// 对接按行为者组织的应用服务层
type PlanHandler struct {
	BaseHandler
	lifecycleService      planApp.PlanLifecycleService
	enrollmentService     planApp.PlanEnrollmentService
	taskSchedulerService  planApp.TaskSchedulerService
	taskManagementService planApp.TaskManagementService
	queryService          planApp.PlanQueryService
}

// NewPlanHandler 创建计划处理器
func NewPlanHandler(
	lifecycleService planApp.PlanLifecycleService,
	enrollmentService planApp.PlanEnrollmentService,
	taskSchedulerService planApp.TaskSchedulerService,
	taskManagementService planApp.TaskManagementService,
	queryService planApp.PlanQueryService,
) *PlanHandler {
	return &PlanHandler{
		lifecycleService:      lifecycleService,
		enrollmentService:     enrollmentService,
		taskSchedulerService:  taskSchedulerService,
		taskManagementService: taskManagementService,
		queryService:          queryService,
	}
}

// ============= Plan Lifecycle API (生命周期管理) =============

// CreatePlan 创建计划
// @Summary 创建测评计划模板
// @Description 创建新的测评计划模板，定义周期策略。需要提供量表编码（scale_code）和周期类型（schedule_type）。不同周期类型需要不同的参数：
// @Description - by_week/by_day: 需要 interval（间隔）和 total_times（总次数）
// @Description - fixed_date: 需要 fixed_dates（固定日期列表）
// @Description - custom: 需要 relative_weeks（相对周次列表）
// @Tags Plan-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.CreatePlanRequest true "创建计划请求（scale_code: 量表编码，如 '3adyDE'）"
// @Success 200 {object} core.Response{data=response.PlanResponse}
// @Router /api/v1/plans [post]
func (h *PlanHandler) CreatePlan(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("CreatePlan handler started",
		"action", "create_plan",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"content_type", c.ContentType(),
		"content_length", c.Request.ContentLength,
	)

	// 记录原始请求体（用于调试）
	if c.Request.Body != nil {
		rawBody, _ := io.ReadAll(c.Request.Body)
		if len(rawBody) > 0 {
			logger.L(ctx).Infow("CreatePlan raw request body",
				"action", "create_plan",
				"raw_body", string(rawBody),
			)
			// 重新设置 Body，因为 ReadAll 会消费掉
			c.Request.Body = io.NopCloser(bytes.NewReader(rawBody))
		}
	}

	var req request.CreatePlanRequest
	logger.L(ctx).Infow("CreatePlan binding JSON",
		"action", "create_plan",
	)
	if err := h.BindJSON(c, &req); err != nil {
		logger.L(ctx).Errorw("CreatePlan BindJSON failed",
			"action", "create_plan",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	logger.L(ctx).Infow("CreatePlan request parsed",
		"action", "create_plan",
		"scale_code", req.ScaleCode,
		"schedule_type", req.ScheduleType,
		"interval", req.Interval,
		"total_times", req.TotalTimes,
		"fixed_dates", req.FixedDates,
		"relative_weeks", req.RelativeWeeks,
	)

	logger.L(ctx).Infow("CreatePlan validating struct",
		"action", "create_plan",
	)
	if ok, err := govalidator.ValidateStruct(req); !ok {
		logger.L(ctx).Errorw("CreatePlan validation failed",
			"action", "create_plan",
			"validation_error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	logger.L(ctx).Infow("CreatePlan validation passed",
		"action", "create_plan",
	)

	// 获取组织ID（从 JWT 中提取，如果为空则使用默认值）
	orgIDUint64 := h.GetOrgIDWithDefault(c)
	orgID := int64(orgIDUint64)
	logger.L(ctx).Infow("CreatePlan got orgID",
		"action", "create_plan",
		"org_id_uint64", orgIDUint64,
		"org_id_int64", orgID,
		"is_default", orgIDUint64 == DefaultOrgID,
	)

	// 根据 schedule_type 验证必需的参数
	logger.L(ctx).Infow("CreatePlan validating schedule_type",
		"action", "create_plan",
		"schedule_type", req.ScheduleType,
	)
	switch req.ScheduleType {
	case "by_week", "by_day":
		logger.L(ctx).Infow("CreatePlan validating by_week/by_day params",
			"action", "create_plan",
			"interval", req.Interval,
			"total_times", req.TotalTimes,
		)
		if req.Interval <= 0 {
			logger.L(ctx).Errorw("CreatePlan interval invalid",
				"action", "create_plan",
				"interval", req.Interval,
			)
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "by_week/by_day 类型需要 interval 参数且必须大于0"))
			return
		}
		if req.TotalTimes <= 0 {
			logger.L(ctx).Errorw("CreatePlan total_times invalid",
				"action", "create_plan",
				"total_times", req.TotalTimes,
			)
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "by_week/by_day 类型需要 total_times 参数且必须大于0"))
			return
		}
	case "fixed_date":
		logger.L(ctx).Infow("CreatePlan validating fixed_date params",
			"action", "create_plan",
			"fixed_dates_count", len(req.FixedDates),
			"fixed_dates", req.FixedDates,
		)
		if len(req.FixedDates) == 0 {
			logger.L(ctx).Errorw("CreatePlan fixed_dates empty",
				"action", "create_plan",
			)
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "fixed_date 类型需要 fixed_dates 参数且不能为空"))
			return
		}
	case "custom":
		logger.L(ctx).Infow("CreatePlan validating custom params",
			"action", "create_plan",
			"relative_weeks_count", len(req.RelativeWeeks),
			"relative_weeks", req.RelativeWeeks,
		)
		if len(req.RelativeWeeks) == 0 {
			logger.L(ctx).Errorw("CreatePlan relative_weeks empty",
				"action", "create_plan",
			)
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "custom 类型需要 relative_weeks 参数且不能为空"))
			return
		}
	default:
		logger.L(ctx).Errorw("CreatePlan invalid schedule_type",
			"action", "create_plan",
			"schedule_type", req.ScheduleType,
		)
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "无效的 schedule_type: %s，支持的类型: by_week, by_day, fixed_date, custom", req.ScheduleType))
		return
	}

	dto := planApp.CreatePlanDTO{
		OrgID:         orgID,
		ScaleCode:     req.ScaleCode,
		ScheduleType:  req.ScheduleType,
		Interval:      req.Interval,
		TotalTimes:    req.TotalTimes,
		FixedDates:    req.FixedDates,
		RelativeWeeks: req.RelativeWeeks,
	}

	logger.L(ctx).Infow("CreatePlan calling lifecycle service",
		"action", "create_plan",
		"dto", dto,
	)

	result, err := h.lifecycleService.CreatePlan(ctx, dto)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan lifecycle service failed",
			"action", "create_plan",
			"resource", "plan",
			"org_id", orgID,
			"dto", dto,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	logger.L(ctx).Infow("CreatePlan success",
		"action", "create_plan",
		"plan_id", result.ID,
		"org_id", orgID,
	)

	h.Success(c, response.NewPlanResponse(result))
}

// PausePlan 暂停计划
// @Summary 暂停计划
// @Description 暂停计划，取消所有未执行的任务
// @Tags Plan-Lifecycle
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "计划ID"
// @Success 200 {object} core.Response{data=response.PlanResponse}
// @Router /api/v1/plans/{id}/pause [post]
func (h *PlanHandler) PausePlan(c *gin.Context) {
	planID := c.Param("id")
	if planID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "计划ID不能为空"))
		return
	}

	result, err := h.lifecycleService.PausePlan(c.Request.Context(), planID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to pause plan",
			"action", "pause_plan",
			"resource", "plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewPlanResponse(result))
}

// ResumePlan 恢复计划
// @Summary 恢复计划
// @Description 恢复计划，重新生成未完成的任务
// @Tags Plan-Lifecycle
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "计划ID"
// @Param request body request.ResumePlanRequest false "恢复计划请求（可选）"
// @Success 200 {object} core.Response{data=response.PlanResponse}
// @Router /api/v1/plans/{id}/resume [post]
func (h *PlanHandler) ResumePlan(c *gin.Context) {
	planID := c.Param("id")
	if planID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "计划ID不能为空"))
		return
	}

	var req request.ResumePlanRequest
	// 可选请求体
	if c.Request.ContentLength > 0 {
		if err := h.BindJSON(c, &req); err != nil {
			h.Error(c, err)
			return
		}
	}

	result, err := h.lifecycleService.ResumePlan(c.Request.Context(), planID, req.TesteeStartDates)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to resume plan",
			"action", "resume_plan",
			"resource", "plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewPlanResponse(result))
}

// CancelPlan 取消计划
// @Summary 取消计划
// @Description 取消计划，不可恢复
// @Tags Plan-Lifecycle
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "计划ID"
// @Success 200 {object} core.Response
// @Router /api/v1/plans/{id}/cancel [post]
func (h *PlanHandler) CancelPlan(c *gin.Context) {
	planID := c.Param("id")
	if planID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "计划ID不能为空"))
		return
	}

	err := h.lifecycleService.CancelPlan(c.Request.Context(), planID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to cancel plan",
			"action", "cancel_plan",
			"resource", "plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "计划已取消", nil)
}

// ============= Plan Enrollment API (受试者加入计划) =============

// EnrollTestee 受试者加入计划
// @Summary 受试者加入计划
// @Description 将受试者加入计划，生成所有任务
// @Tags Plan-Enrollment
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body request.EnrollTesteeRequest true "加入计划请求"
// @Success 200 {object} core.Response{data=response.EnrollmentResponse}
// @Router /api/v1/plans/enroll [post]
func (h *PlanHandler) EnrollTestee(c *gin.Context) {
	var req request.EnrollTesteeRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	dto := planApp.EnrollTesteeDTO{
		PlanID:    req.PlanID,
		TesteeID:  req.TesteeID,
		StartDate: req.StartDate,
	}

	result, err := h.enrollmentService.EnrollTestee(c.Request.Context(), dto)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to enroll testee",
			"action", "enroll_testee",
			"resource", "plan",
			"plan_id", req.PlanID,
			"testee_id", req.TesteeID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewEnrollmentResponse(result))
}

// TerminateEnrollment 终止受试者的计划参与
// @Summary 终止受试者的计划参与
// @Description 受试者退出计划，取消所有待处理任务
// @Tags Plan-Enrollment
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "计划ID"
// @Param testee_id path string true "受试者ID"
// @Success 200 {object} core.Response
// @Router /api/v1/plans/{id}/testees/{testee_id}/terminate [post]
func (h *PlanHandler) TerminateEnrollment(c *gin.Context) {
	planID := c.Param("id")
	testeeID := c.Param("testee_id")

	if planID == "" || testeeID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "计划ID和受试者ID不能为空"))
		return
	}

	err := h.enrollmentService.TerminateEnrollment(c.Request.Context(), planID, testeeID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to terminate enrollment",
			"action", "terminate_enrollment",
			"resource", "plan",
			"plan_id", planID,
			"testee_id", testeeID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "已终止受试者的计划参与", nil)
}

// ============= Task Scheduler API (任务调度) =============

// SchedulePendingTasks 调度待推送任务
// @Summary 调度待推送任务
// @Description 定时任务调用，扫描待推送任务，生成入口并开放
// @Tags Task-Scheduler
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param before query string false "截止时间（格式：YYYY-MM-DD HH:mm:ss），默认当前时间"
// @Success 200 {object} core.Response{data=response.TaskListResponse}
// @Router /api/v1/plans/tasks/schedule [post]
func (h *PlanHandler) SchedulePendingTasks(c *gin.Context) {
	before := c.Query("before")
	if before == "" {
		// 默认使用当前时间
		before = ""
	}

	tasks, err := h.taskSchedulerService.SchedulePendingTasks(c.Request.Context(), before)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to schedule pending tasks",
			"action", "schedule_pending_tasks",
			"resource", "task",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskListResponseFromSlice(tasks))
}

// ============= Task Management API (任务管理) =============

// OpenTask 开放任务
// @Summary 开放任务
// @Description 手动开放任务，生成入口
// @Tags Task-Management
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "任务ID"
// @Param request body request.OpenTaskRequest true "开放任务请求"
// @Success 200 {object} core.Response{data=response.TaskResponse}
// @Router /api/v1/plans/tasks/{id}/open [post]
func (h *PlanHandler) OpenTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "任务ID不能为空"))
		return
	}

	var req request.OpenTaskRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	dto := planApp.OpenTaskDTO{
		EntryToken: req.EntryToken,
		EntryURL:   req.EntryURL,
		ExpireAt:   req.ExpireAt,
	}

	result, err := h.taskManagementService.OpenTask(c.Request.Context(), taskID, dto)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to open task",
			"action", "open_task",
			"resource", "task",
			"task_id", taskID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskResponse(result))
}

// CompleteTask 完成任务
// @Summary 完成任务
// @Description 用户完成测评后，更新任务状态
// @Tags Task-Management
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "任务ID"
// @Param assessment_id query string true "测评ID"
// @Success 200 {object} core.Response{data=response.TaskResponse}
// @Router /api/v1/plans/tasks/{id}/complete [post]
func (h *PlanHandler) CompleteTask(c *gin.Context) {
	taskID := c.Param("id")
	assessmentID := c.Query("assessment_id")

	if taskID == "" || assessmentID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "任务ID和测评ID不能为空"))
		return
	}

	result, err := h.taskManagementService.CompleteTask(c.Request.Context(), taskID, assessmentID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to complete task",
			"action", "complete_task",
			"resource", "task",
			"task_id", taskID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskResponse(result))
}

// ExpireTask 过期任务
// @Summary 过期任务
// @Description 定时任务调用，标记已过期的任务
// @Tags Task-Management
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "任务ID"
// @Success 200 {object} core.Response{data=response.TaskResponse}
// @Router /api/v1/plans/tasks/{id}/expire [post]
func (h *PlanHandler) ExpireTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "任务ID不能为空"))
		return
	}

	result, err := h.taskManagementService.ExpireTask(c.Request.Context(), taskID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to expire task",
			"action", "expire_task",
			"resource", "task",
			"task_id", taskID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskResponse(result))
}

// CancelTask 取消任务
// @Summary 取消任务
// @Description 手动取消任务
// @Tags Task-Management
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "任务ID"
// @Success 200 {object} core.Response
// @Router /api/v1/plans/tasks/{id}/cancel [post]
func (h *PlanHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "任务ID不能为空"))
		return
	}

	err := h.taskManagementService.CancelTask(c.Request.Context(), taskID)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("Failed to cancel task",
			"action", "cancel_task",
			"resource", "task",
			"task_id", taskID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.SuccessResponseWithMessage(c, "任务已取消", nil)
}

// ============= Query API (查询) =============

// GetPlan 获取计划详情
// @Summary 获取计划详情
// @Description 查询指定计划的完整信息
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "计划ID"
// @Success 200 {object} core.Response{data=response.PlanResponse}
// @Router /api/v1/plans/{id} [get]
func (h *PlanHandler) GetPlan(c *gin.Context) {
	planID := c.Param("id")
	if planID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "计划ID不能为空"))
		return
	}

	result, err := h.queryService.GetPlan(c.Request.Context(), planID)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewPlanResponse(result))
}

// ListPlans 查询计划列表
// @Summary 查询计划列表
// @Description 分页查询计划列表，支持条件筛选。可通过量表编码（scale_code）筛选特定量表的计划
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param org_id query int false "机构ID"
// @Param scale_code query string false "量表编码（如 '3adyDE'）"
// @Param status query string false "状态（active/paused/finished/canceled）"
// @Param page query int true "页码（从1开始）"
// @Param page_size query int true "每页数量"
// @Success 200 {object} core.Response{data=response.PlanListResponse}
// @Router /api/v1/plans [get]
func (h *PlanHandler) ListPlans(c *gin.Context) {
	var req request.ListPlansRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	// 如果没有指定 org_id，从 JWT 中获取，如果为空则使用默认值
	if req.OrgID == 0 {
		req.OrgID = int64(h.GetOrgIDWithDefault(c))
	}

	dto := planApp.ListPlansDTO{
		OrgID:     req.OrgID,
		ScaleCode: req.ScaleCode,
		Status:    req.Status,
		Page:      req.Page,
		PageSize:  req.PageSize,
	}

	result, err := h.queryService.ListPlans(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewPlanListResponse(result))
}

// GetTask 获取任务详情
// @Summary 获取任务详情
// @Description 查询指定任务的完整信息
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "任务ID"
// @Success 200 {object} core.Response{data=response.TaskResponse}
// @Router /api/v1/plans/tasks/{id} [get]
func (h *PlanHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "任务ID不能为空"))
		return
	}

	result, err := h.queryService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskResponse(result))
}

// ListTasks 查询任务列表
// @Summary 查询任务列表
// @Description 分页查询任务列表，支持条件筛选
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param plan_id query string false "计划ID"
// @Param testee_id query string false "受试者ID"
// @Param status query string false "状态"
// @Param page query int true "页码"
// @Param page_size query int true "每页数量"
// @Success 200 {object} core.Response{data=response.TaskListResponse}
// @Router /api/v1/plans/tasks [get]
func (h *PlanHandler) ListTasks(c *gin.Context) {
	var req request.ListTasksRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	dto := planApp.ListTasksDTO{
		PlanID:   req.PlanID,
		TesteeID: req.TesteeID,
		Status:   req.Status,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	result, err := h.queryService.ListTasks(c.Request.Context(), dto)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskListResponse(result))
}

// ListTasksByPlan 查询计划下的所有任务
// @Summary 查询计划下的所有任务
// @Description 查看某个计划的所有任务
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "计划ID"
// @Success 200 {object} core.Response{data=response.TaskListResponse}
// @Router /api/v1/plans/{id}/tasks [get]
func (h *PlanHandler) ListTasksByPlan(c *gin.Context) {
	planID := c.Param("id")
	if planID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "计划ID不能为空"))
		return
	}

	tasks, err := h.queryService.ListTasksByPlan(c.Request.Context(), planID)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskListResponseFromSlice(tasks))
}

// ListTasksByTestee 查询受试者的所有任务
// @Summary 查询受试者的所有任务
// @Description 查看某个受试者的所有任务
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "受试者ID"
// @Success 200 {object} core.Response{data=response.TaskListResponse}
// @Router /api/v1/testees/{id}/tasks [get]
func (h *PlanHandler) ListTasksByTestee(c *gin.Context) {
	testeeID := c.Param("id")
	if testeeID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "受试者ID不能为空"))
		return
	}

	tasks, err := h.queryService.ListTasksByTestee(c.Request.Context(), testeeID)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskListResponseFromSlice(tasks))
}

// ListPlansByTestee 查询受试者参与的所有计划
// @Summary 查询受试者参与的所有计划
// @Description 查看某个受试者参与的所有计划
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "受试者ID"
// @Success 200 {object} core.Response{data=response.PlanListResponse}
// @Router /api/v1/testees/{id}/plans [get]
func (h *PlanHandler) ListPlansByTestee(c *gin.Context) {
	testeeID := c.Param("id")
	if testeeID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "受试者ID不能为空"))
		return
	}

	plans, err := h.queryService.ListPlansByTestee(c.Request.Context(), testeeID)
	if err != nil {
		h.Error(c, err)
		return
	}

	// 转换为列表响应格式
	planResponses := make([]response.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		if resp := response.NewPlanResponse(plan); resp != nil {
			planResponses = append(planResponses, *resp)
		}
	}

	h.Success(c, &response.PlanListResponse{
		Plans:      planResponses,
		TotalCount: int64(len(planResponses)),
		Page:       1,
		PageSize:   len(planResponses),
	})
}

// ListTasksByTesteeAndPlan 查询受试者在某个计划下的所有任务
// @Summary 查询受试者在某个计划下的所有任务
// @Description 查看某个受试者在某个计划下的所有任务
// @Tags Plan-Query
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path string true "受试者ID"
// @Param plan_id path string true "计划ID"
// @Success 200 {object} core.Response{data=response.TaskListResponse}
// @Router /api/v1/testees/{id}/plans/{plan_id}/tasks [get]
func (h *PlanHandler) ListTasksByTesteeAndPlan(c *gin.Context) {
	testeeID := c.Param("id")
	planID := c.Param("plan_id")

	if testeeID == "" || planID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "受试者ID和计划ID不能为空"))
		return
	}

	tasks, err := h.queryService.ListTasksByTesteeAndPlan(c.Request.Context(), testeeID, planID)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.NewTaskListResponseFromSlice(tasks))
}
