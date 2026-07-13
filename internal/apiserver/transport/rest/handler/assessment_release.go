package handler

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/gin-gonic/gin"
)

// AssessmentReleaseHandler exposes the only public publish/archive boundary
// for questionnaire-backed assessment models.
type AssessmentReleaseHandler struct {
	BaseHandler
	service modelcatalog.AssessmentReleaseService
}

func NewAssessmentReleaseHandler(service modelcatalog.AssessmentReleaseService) *AssessmentReleaseHandler {
	return &AssessmentReleaseHandler{service: service}
}

// Publish atomically publishes the model and its bound questionnaire.
// @Summary 发布测评版本
// @Tags AssessmentRelease
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=modelcatalog.AssessmentRelease}
// @Router /api/v1/assessment-releases/{code}/publish [post]
func (h *AssessmentReleaseHandler) Publish(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.PublishRelease(c.Request.Context(), actor, c.Param("code"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// Archive atomically archives the active model/questionnaire pair.
// @Summary 归档测评版本
// @Tags AssessmentRelease
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=modelcatalog.AssessmentRelease}
// @Router /api/v1/assessment-releases/{code}/archive [post]
func (h *AssessmentReleaseHandler) Archive(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.ArchiveRelease(c.Request.Context(), actor, c.Param("code"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}
