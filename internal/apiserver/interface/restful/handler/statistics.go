package handler

import (
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
	screeningStatisticsService     statisticsApp.ScreeningStatisticsService
	syncService                    statisticsApp.StatisticsSyncService
	validatorService               statisticsApp.StatisticsValidatorService
}

// NewStatisticsHandler 创建统计处理器
func NewStatisticsHandler(
	systemStatisticsService statisticsApp.SystemStatisticsService,
	questionnaireStatisticsService statisticsApp.QuestionnaireStatisticsService,
	testeeStatisticsService statisticsApp.TesteeStatisticsService,
	planStatisticsService statisticsApp.PlanStatisticsService,
	screeningStatisticsService statisticsApp.ScreeningStatisticsService,
	syncService statisticsApp.StatisticsSyncService,
	validatorService statisticsApp.StatisticsValidatorService,
) *StatisticsHandler {
	return &StatisticsHandler{
		systemStatisticsService:        systemStatisticsService,
		questionnaireStatisticsService: questionnaireStatisticsService,
		testeeStatisticsService:        testeeStatisticsService,
		planStatisticsService:          planStatisticsService,
		screeningStatisticsService:     screeningStatisticsService,
		syncService:                    syncService,
		validatorService:               validatorService,
	}
}

// ============= 统计查询 API =============

// GetSystemStatistics 获取系统整体统计
// @Summary 获取系统整体统计
// @Description 获取系统整体统计数据，包括问卷数量、答卷数量、受试者数量等
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=statistics.SystemStatistics}
// @Router /api/v1/statistics/system [get]
func (h *StatisticsHandler) GetSystemStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("获取系统整体统计", "action", "get_system_statistics")

	orgID := int64(h.GetOrgIDWithDefault(c))

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
// @Description 获取指定问卷/量表的统计数据，包括总提交数、完成数、趋势等
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "问卷编码"
// @Success 200 {object} core.Response{data=statistics.QuestionnaireStatistics}
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

	orgID := int64(h.GetOrgIDWithDefault(c))

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
// @Description 获取指定受试者的统计数据，包括测评数、完成数、风险分布等
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param testee_id path uint64 true "受试者ID"
// @Success 200 {object} core.Response{data=statistics.TesteeStatistics}
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

	orgID := int64(h.GetOrgIDWithDefault(c))

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
// @Description 获取指定测评计划的统计数据，包括任务数、完成率等
// @Tags Statistics
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param plan_id path uint64 true "计划ID"
// @Success 200 {object} core.Response{data=statistics.PlanStatistics}
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

	orgID := int64(h.GetOrgIDWithDefault(c))

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

// SyncDailyStatistics 同步每日统计（定时任务调用）
// @Summary 同步每日统计
// @Description 将Redis中的每日统计数据同步到MySQL（定时任务调用）
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Router /api/v1/statistics/sync/daily [post]
func (h *StatisticsHandler) SyncDailyStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("同步每日统计", "action", "sync_daily_statistics")

	if err := h.syncService.SyncDailyStatistics(ctx); err != nil {
		logger.L(ctx).Errorw("同步每日统计失败",
			"action", "sync_daily_statistics",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "每日统计同步完成"})
}

// SyncAccumulatedStatistics 同步累计统计（定时任务调用）
// @Summary 同步累计统计
// @Description 将Redis中的累计统计数据同步到MySQL（定时任务调用）
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Router /api/v1/statistics/sync/accumulated [post]
func (h *StatisticsHandler) SyncAccumulatedStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("同步累计统计", "action", "sync_accumulated_statistics")

	if err := h.syncService.SyncAccumulatedStatistics(ctx); err != nil {
		logger.L(ctx).Errorw("同步累计统计失败",
			"action", "sync_accumulated_statistics",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "累计统计同步完成"})
}

// SyncPlanStatistics 同步计划统计（定时任务调用）
// @Summary 同步计划统计
// @Description 同步计划统计数据到MySQL（定时任务调用）
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Router /api/v1/statistics/sync/plan [post]
func (h *StatisticsHandler) SyncPlanStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("同步计划统计", "action", "sync_plan_statistics")

	if err := h.syncService.SyncPlanStatistics(ctx); err != nil {
		logger.L(ctx).Errorw("同步计划统计失败",
			"action", "sync_plan_statistics",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "计划统计同步完成"})
}

// ValidateConsistency 校验数据一致性（定时任务调用）
// @Summary 校验数据一致性
// @Description 校验Redis和MySQL统计数据的一致性，修复不一致（定时任务调用）
// @Tags Statistics-Sync
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response
// @Router /api/v1/statistics/validate [post]
func (h *StatisticsHandler) ValidateConsistency(c *gin.Context) {
	ctx := c.Request.Context()
	logger.L(ctx).Infow("校验数据一致性", "action", "validate_consistency")

	if err := h.validatorService.ValidateConsistency(ctx); err != nil {
		logger.L(ctx).Errorw("校验数据一致性失败",
			"action", "validate_consistency",
			"error", err.Error(),
		)
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "数据一致性校验完成"})
}
