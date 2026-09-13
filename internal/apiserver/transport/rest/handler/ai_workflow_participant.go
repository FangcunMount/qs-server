package handler

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type AIWorkflowParticipantHandler struct {
	*BaseHandler
	service *app.ParticipantAdministration
}

func NewAIWorkflowParticipantHandler(service *app.ParticipantAdministration) *AIWorkflowParticipantHandler {
	return &AIWorkflowParticipantHandler{BaseHandler: NewBaseHandler(), service: service}
}

// Capacity godoc
// @Summary 查询 qs-ai 参与者生成容量
// @Description 当前机构 OrgAdmin 权限。三级额度和活动名额由 AI 计算；过滤参数只选择查询对象，不更改调用身份。查询不会启动或重试生成。
// @Tags AI-Workflow-Management
// @Produce json
// @Param subject_id query string false "可选的原参与者主体标识"
// @Param assessment_id query string false "可选的原测评 ID"
// @Success 200 {object} core.Response{data=app.ParticipantCapacity}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/participant-capacity [get]
func (h *AIWorkflowParticipantHandler) Capacity(c *gin.Context) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	failure := NewAIWorkflowManagementHandler(nil)
	values := c.Request.URL.Query()
	if len(c.Request.URL.RawQuery) > 2048 || len(values["subject_id"]) > 1 || len(values["assessment_id"]) > 1 {
		failure.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.Capacity(c.Request.Context(), app.DraftScope{OrganizationID: org, OperatorUserID: user}, app.ParticipantCapacityQuery{SubjectID: values.Get("subject_id"), AssessmentID: values.Get("assessment_id")})
	if err != nil {
		failure.failure(c, err)
		return
	}
	h.Success(c, value)
}
