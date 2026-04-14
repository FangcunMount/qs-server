package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// StatisticsHandler 统计处理器
type StatisticsHandler struct {
	BaseHandler
	systemStatisticsService        statisticsApp.SystemStatisticsService
	questionnaireStatisticsService statisticsApp.QuestionnaireStatisticsService
	testeeStatisticsService        statisticsApp.TesteeStatisticsService
	planStatisticsService          statisticsApp.PlanStatisticsService
	readService                    statisticsApp.ReadService
	periodicStatsService           statisticsApp.PeriodicStatsService
	syncService                    statisticsApp.StatisticsSyncService
	validatorService               statisticsApp.StatisticsValidatorService
	testeeAccessService            actorAccessApp.TesteeAccessService
}

// NewStatisticsHandler 创建统计处理器
func NewStatisticsHandler(
	systemStatisticsService statisticsApp.SystemStatisticsService,
	questionnaireStatisticsService statisticsApp.QuestionnaireStatisticsService,
	testeeStatisticsService statisticsApp.TesteeStatisticsService,
	planStatisticsService statisticsApp.PlanStatisticsService,
	readService statisticsApp.ReadService,
	periodicStatsService statisticsApp.PeriodicStatsService,
	syncService statisticsApp.StatisticsSyncService,
	validatorService statisticsApp.StatisticsValidatorService,
) *StatisticsHandler {
	return &StatisticsHandler{
		systemStatisticsService:        systemStatisticsService,
		questionnaireStatisticsService: questionnaireStatisticsService,
		testeeStatisticsService:        testeeStatisticsService,
		planStatisticsService:          planStatisticsService,
		readService:                    readService,
		periodicStatsService:           periodicStatsService,
		syncService:                    syncService,
		validatorService:               validatorService,
	}
}

// SetTesteeAccessService 设置 testee 访问控制服务。
func (h *StatisticsHandler) SetTesteeAccessService(testeeAccessService actorAccessApp.TesteeAccessService) {
	h.testeeAccessService = testeeAccessService
}

func (h *StatisticsHandler) bindJSON(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid request body: %v", err))
		return false
	}
	return true
}

func (h *StatisticsHandler) parsePage(c *gin.Context) (int, int, error) {
	page := 1
	pageSize := 20
	if raw := c.Query("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.WithCode(code.ErrInvalidArgument, "invalid page: %s", raw)
		}
		page = value
	}
	if raw := c.Query("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.WithCode(code.ErrInvalidArgument, "invalid page_size: %s", raw)
		}
		pageSize = value
	}
	return page, pageSize, nil
}

func buildStatisticsQueryFilter(c *gin.Context) statisticsApp.QueryFilter {
	return statisticsApp.QueryFilter{
		Preset: c.Query("preset"),
		From:   c.Query("from"),
		To:     c.Query("to"),
	}
}

type questionnaireBatchRequest struct {
	Codes []string `json:"codes"`
}

func (h *StatisticsHandler) GetOverview(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	stats, err := h.readService.GetOverview(ctx, orgID, buildStatisticsQueryFilter(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) ListClinicianStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	page, pageSize, err := h.parsePage(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	stats, err := h.readService.ListClinicianStatistics(ctx, orgID, buildStatisticsQueryFilter(c), page, pageSize)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) GetClinicianStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	clinicianID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid clinician id: %s", c.Param("id")))
		return
	}
	stats, err := h.readService.GetClinicianStatistics(ctx, orgID, clinicianID, buildStatisticsQueryFilter(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) ListAssessmentEntryStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	page, pageSize, err := h.parsePage(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var clinicianID *uint64
	if raw := c.Query("clinician_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid clinician_id: %s", raw))
			return
		}
		clinicianID = &value
	}
	var activeOnly *bool
	if raw := c.Query("status"); raw != "" {
		value := raw == "active"
		activeOnly = &value
	}
	stats, err := h.readService.ListAssessmentEntryStatistics(ctx, orgID, clinicianID, activeOnly, buildStatisticsQueryFilter(c), page, pageSize)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) GetAssessmentEntryStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	entryID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid entry id: %s", c.Param("id")))
		return
	}
	stats, err := h.readService.GetAssessmentEntryStatistics(ctx, orgID, entryID, buildStatisticsQueryFilter(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) GetCurrentClinicianOverview(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	stats, err := h.readService.GetCurrentClinicianStatistics(ctx, orgID, operatorUserID, buildStatisticsQueryFilter(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) ListCurrentClinicianEntryStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	page, pageSize, err := h.parsePage(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	stats, err := h.readService.ListCurrentClinicianEntryStatistics(ctx, orgID, operatorUserID, buildStatisticsQueryFilter(c), page, pageSize)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) GetCurrentClinicianTesteeSummary(c *gin.Context) {
	ctx := c.Request.Context()
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	stats, err := h.readService.GetCurrentClinicianTesteeSummary(ctx, orgID, operatorUserID, buildStatisticsQueryFilter(c))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) GetTesteePeriodicStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	testeeID, err := strconv.ParseUint(c.Param("testee_id"), 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "invalid testee id: %s", c.Param("testee_id")))
		return
	}
	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID); err != nil {
		h.Error(c, err)
		return
	}
	stats, err := h.periodicStatsService.GetPeriodicStats(ctx, orgID, testeeID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

func (h *StatisticsHandler) BatchQuestionnaireStatistics(c *gin.Context) {
	var req questionnaireBatchRequest
	if !h.bindJSON(c, &req) {
		return
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	stats, err := h.readService.GetQuestionnaireBatchStatistics(c.Request.Context(), orgID, req.Codes)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, stats)
}

// ============= 统计查询 API =============

// GetSystemStatistics 获取系统整体统计
// @Summary 获取系统整体统计
// @Description 获取系统整体统计数据，包括问卷数量、答卷数量、受试者数量等；仅 qs:admin 可访问
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=statistics.SystemStatistics}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/statistics/system [get]
func (h *StatisticsHandler) GetSystemStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("获取系统整体统计", "action", "get_system_statistics")

	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	stats, err := h.systemStatisticsService.GetSystemStatistics(ctx, orgID)
	if err != nil {
		logger.L(ctx).Errorw("获取系统整体统计失败",
			"action", "get_system_statistics",
			"org_id", orgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, stats)
}

// GetQuestionnaireStatistics 获取问卷/量表统计
// @Summary 获取问卷/量表统计
// @Description 获取指定问卷/量表的统计数据，包括总提交数、完成数、趋势等；仅 qs:admin 可访问
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} core.Response{data=statistics.QuestionnaireStatistics}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/statistics/questionnaires/{code} [get]
func (h *StatisticsHandler) GetQuestionnaireStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	questionnaireCode := c.Param("code")
	logger.L(ctx).Infow("获取问卷统计",
		"action", "get_questionnaire_statistics",
		"questionnaire_code", questionnaireCode,
	)

	if questionnaireCode == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "问卷编码不能为空"))
		return
	}

	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	stats, err := h.questionnaireStatisticsService.GetQuestionnaireStatistics(ctx, orgID, questionnaireCode)
	if err != nil {
		logger.L(ctx).Errorw("获取问卷统计失败",
			"action", "get_questionnaire_statistics",
			"org_id", orgID,
			"questionnaire_code", questionnaireCode,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, stats)
}

// GetTesteeStatistics 获取受试者统计
// @Summary 获取受试者统计
// @Description 获取指定受试者的统计数据，包括测评数、完成数、风险分布等；后台访问范围按 ClinicianTesteeRelation 收口
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param testee_id path uint64 true "受试者ID"
// @Success 200 {object} core.Response{data=statistics.TesteeStatistics}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/statistics/testees/{testee_id} [get]
func (h *StatisticsHandler) GetTesteeStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	testeeIDStr := c.Param("testee_id")
	logger.L(ctx).Infow("获取受试者统计",
		"action", "get_testee_statistics",
		"testee_id", testeeIDStr,
	)

	testeeID, err := strconv.ParseUint(testeeIDStr, 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "无效的受试者ID: %s", testeeIDStr))
		return
	}

	orgID, operatorUserID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	if err := h.testeeAccessService.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID); err != nil {
		h.Error(c, err)
		return
	}

	stats, err := h.testeeStatisticsService.GetTesteeStatistics(ctx, orgID, testeeID)
	if err != nil {
		logger.L(ctx).Errorw("获取受试者统计失败",
			"action", "get_testee_statistics",
			"org_id", orgID,
			"testee_id", testeeID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, stats)
}

// GetPlanStatistics 获取计划统计
// @Summary 获取计划统计
// @Description 获取指定测评计划的统计数据，包括任务数、完成率等；仅 qs:admin 可访问
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param plan_id path uint64 true "计划ID"
// @Success 200 {object} core.Response{data=statistics.PlanStatistics}
// @Failure 429 {object} core.ErrResponse
// @Router /api/v1/statistics/plans/{plan_id} [get]
func (h *StatisticsHandler) GetPlanStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	planIDStr := c.Param("plan_id")
	planID, err := strconv.ParseUint(planIDStr, 10, 64)
	if err != nil {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "无效的计划ID: %s", planIDStr))
		return
	}

	logger.L(ctx).Infow("获取计划统计",
		"action", "get_plan_statistics",
		"plan_id", planID,
	)

	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	stats, err := h.planStatisticsService.GetPlanStatistics(ctx, orgID, planID)
	if err != nil {
		logger.L(ctx).Errorw("获取计划统计失败",
			"action", "get_plan_statistics",
			"org_id", orgID,
			"plan_id", planID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, stats)
}

// ============= 定时任务 API =============

// SyncDailyStatistics 同步每日统计（内部系统动作）
// @Summary 同步每日统计
// @Description 将Redis中的每日统计数据同步到MySQL（定时任务调用）；仅 qs:admin 可访问
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/statistics/sync/daily [post]
func (h *StatisticsHandler) SyncDailyStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("同步每日统计", "action", "sync_daily_statistics")
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	if err := h.syncService.SyncDailyStatistics(ctx, orgID); err != nil {
		logger.L(ctx).Errorw("同步每日统计失败",
			"action", "sync_daily_statistics",
			"org_id", orgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "每日统计同步完成"})
}

// SyncAccumulatedStatistics 同步累计统计（定时任务调用）
// @Summary 同步累计统计
// @Description 将Redis中的累计统计数据同步到MySQL（定时任务调用）；仅 qs:admin 可访问
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/statistics/sync/accumulated [post]
func (h *StatisticsHandler) SyncAccumulatedStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("同步累计统计", "action", "sync_accumulated_statistics")
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	if err := h.syncService.SyncAccumulatedStatistics(ctx, orgID); err != nil {
		logger.L(ctx).Errorw("同步累计统计失败",
			"action", "sync_accumulated_statistics",
			"org_id", orgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "累计统计同步完成"})
}

// SyncPlanStatistics 同步计划统计（定时任务调用）
// @Summary 同步计划统计
// @Description 同步计划统计数据到MySQL（定时任务调用）；仅 qs:admin 可访问
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/statistics/sync/plan [post]
func (h *StatisticsHandler) SyncPlanStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("同步计划统计", "action", "sync_plan_statistics")
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	if err := h.syncService.SyncPlanStatistics(ctx, orgID); err != nil {
		logger.L(ctx).Errorw("同步计划统计失败",
			"action", "sync_plan_statistics",
			"org_id", orgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "计划统计同步完成"})
}

// ValidateConsistency 校验数据一致性（定时任务调用）
// @Summary 校验数据一致性
// @Description 校验Redis和MySQL统计数据的一致性，修复不一致（定时任务调用）；仅 qs:admin 可访问
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/statistics/validate [post]
func (h *StatisticsHandler) ValidateConsistency(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("校验数据一致性", "action", "validate_consistency")
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}

	if err := h.validatorService.ValidateConsistency(ctx, orgID); err != nil {
		logger.L(ctx).Errorw("校验数据一致性失败",
			"action", "validate_consistency",
			"org_id", orgID,
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "数据一致性校验完成"})
}
