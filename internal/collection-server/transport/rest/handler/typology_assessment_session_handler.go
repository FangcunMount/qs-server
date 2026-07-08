package handler

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologysession"
	"github.com/gin-gonic/gin"
)

type TypologyAssessmentSessionHandler struct {
	*BaseHandler
	service *typologysession.Service
}

func NewTypologyAssessmentSessionHandler(service *typologysession.Service) *TypologyAssessmentSessionHandler {
	return &TypologyAssessmentSessionHandler{
		BaseHandler: NewBaseHandler(),
		service:     service,
	}
}

// Start starts a stateless typology assessment session for mini-program clients.
// @Summary 开始类型学测评会话
// @Description 小程序推荐入口。根据 model_code 聚合返回模型摘要、精确题版问卷、答卷提交契约与后续查询端点模板；不提前创建测评。推荐流程：session → POST /answersheets → GET /answersheets/{id}/assessment → wait-report → report。
// @Tags 类型学测评
// @Accept json
// @Produce json
// @Param body body typologysession.StartSessionRequest true "开始会话请求"
// @Success 200 {object} core.Response{data=typologysession.StartSessionResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /api/v1/typology-assessment-sessions [post]
func (h *TypologyAssessmentSessionHandler) Start(c *gin.Context) {
	var req typologysession.StartSessionRequest
	if err := h.BindJSON(c, &req); err != nil {
		return
	}
	if req.ModelCode == "" || req.TesteeID == 0 {
		h.BadRequestResponse(c, "model_code and testee_id are required", nil)
		return
	}
	result, err := h.service.Start(c.Request.Context(), &req)
	if err != nil {
		h.InternalErrorResponse(c, "start typology assessment session failed", err)
		return
	}
	if result == nil {
		h.NotFoundResponse(c, "typology model not found", nil)
		return
	}
	h.Success(c, result)
}
