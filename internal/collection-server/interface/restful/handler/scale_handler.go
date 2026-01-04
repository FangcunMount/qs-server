package handler

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/gin-gonic/gin"
)

// ScaleHandler 量表处理器
type ScaleHandler struct {
	*BaseHandler
	queryService *scale.QueryService
}

// NewScaleHandler 创建量表处理器
func NewScaleHandler(queryService *scale.QueryService) *ScaleHandler {
	return &ScaleHandler{
		BaseHandler:  NewBaseHandler(),
		queryService: queryService,
	}
}

// Get 获取量表详情
// @Summary 获取量表详情
// @Description 根据量表编码获取量表详情。注意：返回的因子列表只包含 is_show = true 的因子。
// @Description 响应字段说明：
// @Description - category: 主类（adhd/tic/sensory/executive/mental/neurodev/chronic/qol）
// @Description - stages: 阶段列表（数组，screening/deep_assessment/follow_up/outcome）
// @Description - applicable_ages: 使用年龄列表（数组，infant/preschool/school_child/adolescent/adult）
// @Description - reporters: 填报人列表（数组，可包含 parent/teacher/self/clinical）
// @Description - tags: 标签列表（数组，动态输入）
// @Description - question_count: 题目数量（不包含 Section 题型）
// @Description - factors: 因子列表（只包含 is_show = true 的因子），每个因子包含 max_score（最大分，可选）和 is_show（是否显示）字段
// @Tags 量表
// @Produce json
// @Param code path string true "量表编码"
// @Success 200 {object} core.Response{data=scale.ScaleResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/scales/{code} [get]
func (h *ScaleHandler) Get(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		h.BadRequestResponse(c, "code is required", nil)
		return
	}

	result, err := h.queryService.Get(c.Request.Context(), code)
	if err != nil {
		h.InternalErrorResponse(c, "get scale failed", err)
		return
	}

	if result == nil {
		h.NotFoundResponse(c, "scale not found", nil)
		return
	}

	h.Success(c, result)
}

// List 获取量表列表
// @Summary 获取量表列表
// @Description 分页获取量表列表（摘要信息，不包含因子详情），支持按主类、阶段、使用年龄、填报人、标签等条件过滤。
// @Description 查询参数说明：
// @Description - category: 主类过滤，可选值：adhd/tic/sensory/executive/mental/neurodev/chronic/qol
// @Description - stages: 阶段过滤（数组），可选值：screening/deep_assessment/follow_up/outcome
// @Description - applicable_ages: 使用年龄过滤（数组），可选值：infant/preschool/school_child/adolescent/adult
// @Description - reporters: 填报人过滤（数组），可选值：parent/teacher/self/clinical
// @Description - tags: 标签过滤（数组），动态标签值
// @Description 响应中包含分类字段：category、stages（数组）、applicable_ages（数组）、reporters（数组）、tags（数组）、question_count（题目数量，不包含 Section 题型）
// @Tags 量表
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query string false "状态过滤（draft/published/archived）"
// @Param title query string false "标题过滤"
// @Param category query string false "主类过滤"
// @Param stages query []string false "阶段过滤（数组）"
// @Param applicable_ages query []string false "使用年龄过滤（数组）"
// @Param reporters query []string false "填报人过滤（数组）"
// @Param tags query []string false "标签过滤（数组）"
// @Success 200 {object} core.Response{data=scale.ListScalesResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/scales [get]
func (h *ScaleHandler) List(c *gin.Context) {
	var req scale.ListScalesRequest
	if err := h.BindQuery(c, &req); err != nil {
		return
	}

	result, err := h.queryService.List(c.Request.Context(), &req)
	if err != nil {
		h.InternalErrorResponse(c, "list scales failed", err)
		return
	}

	h.Success(c, result)
}

// GetCategories 获取量表分类列表
// @Summary 获取量表分类列表
// @Description 获取量表的主类、阶段、使用年龄、填报人等分类选项列表，用于前端渲染和配置量表字段。
// @Description 返回说明：
// @Description - categories: 主类列表，包含8个选项（adhd, tic, sensory, executive, mental, neurodev, chronic, qol）
// @Description - stages: 阶段列表，包含4个选项（screening, deep_assessment, follow_up, outcome）
// @Description - applicable_ages: 使用年龄列表，包含5个选项（infant, preschool, school_child, adolescent, adult）
// @Description - reporters: 填报人列表，包含4个选项（parent, teacher, self, clinical）
// @Description - tags: 标签列表，返回空数组（标签已改为动态输入，通过后台输入设置）
// @Tags 量表
// @Produce json
// @Success 200 {object} core.Response{data=scale.ScaleCategoriesResponse}
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/scales/categories [get]
func (h *ScaleHandler) GetCategories(c *gin.Context) {
	result, err := h.queryService.GetCategories(c.Request.Context())
	if err != nil {
		h.InternalErrorResponse(c, "get scale categories failed", err)
		return
	}

	h.Success(c, result)
}
